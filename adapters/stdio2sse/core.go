package stdio2sse

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StdioToSSEAdapter 将 stdio MCP 服务器适配为 SSE 服务器
type StdioToSSEAdapter struct {
	command     string
	env         []string
	args        []string
	mcpServer   *server.MCPServer
	sseServer   *server.SSEServer
	stdioClient *client.Client
	mutex       sync.RWMutex
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// AdapterOption 配置选项
type AdapterOption func(*StdioToSSEAdapter)

// WithEnvironment 设置环境变量
func WithEnvironment(env []string) AdapterOption {
	return func(a *StdioToSSEAdapter) {
		a.env = env
	}
}

// NewStdioToSSEAdapter 创建新的适配器实例
func NewStdioToSSEAdapter(command string, args []string, opts ...AdapterOption) *StdioToSSEAdapter {
	ctx, cancel := context.WithCancel(context.Background())

	adapter := &StdioToSSEAdapter{
		command: command,
		args:    args,
		env:     []string{},
		ctx:     ctx,
		cancel:  cancel,
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// Initialize 初始化适配器，建立 stdio 连接并创建 SSE 服务器
func (a *StdioToSSEAdapter) Initialize() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return fmt.Errorf("adapter is already initialized")
	}

	// 创建 stdio 客户端
	stdioClient, err := client.NewStdioMCPClient(a.command, a.env, a.args...)
	if err != nil {
		return fmt.Errorf("failed to create stdio client: %w", err)
	}

	// 启动客户端
	if err := stdioClient.Start(a.ctx); err != nil {
		stdioClient.Close()
		return fmt.Errorf("failed to start stdio client: %w", err)
	}

	// 初始化连接
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "stdio2sse-adapter",
		Version: "1.0.0",
	}

	serverInfo, err := stdioClient.Initialize(a.ctx, initRequest)
	if err != nil {
		stdioClient.Close()
		return fmt.Errorf("failed to initialize stdio connection: %w", err)
	}

	a.stdioClient = stdioClient

	// 创建 MCP 服务器实例
	a.mcpServer = server.NewMCPServer(
		serverInfo.ServerInfo.Name,
		serverInfo.ServerInfo.Version,
		server.WithInstructions("This server is proxied from a stdio MCP server"),
	)

	// 代理工具功能
	if err := a.proxyTools(); err != nil {
		a.cleanup()
		return fmt.Errorf("failed to proxy tools: %w", err)
	}

	// 代理资源功能
	if err := a.proxyResources(); err != nil {
		a.cleanup()
		return fmt.Errorf("failed to proxy resources: %w", err)
	}

	// 代理提示功能
	if err := a.proxyPrompts(); err != nil {
		a.cleanup()
		return fmt.Errorf("failed to proxy prompts: %w", err)
	}

	// 创建 SSE 服务器
	a.sseServer = server.NewSSEServer(a.mcpServer,
		server.WithKeepAlive(true),
		server.WithKeepAliveInterval(30*time.Second),
	)

	a.running = true
	return nil
}

// proxyTools 代理工具功能
func (a *StdioToSSEAdapter) proxyTools() error {
	toolsRequest := mcp.ListToolsRequest{}
	toolsResult, err := a.stdioClient.ListTools(a.ctx, toolsRequest)
	if err != nil {
		// return fmt.Errorf("failed to list tools from stdio server: %w", err)
	}

	for _, tool := range toolsResult.Tools {
		toolCopy := tool // 捕获循环变量

		handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// 转发工具调用到 stdio 服务器
			return a.stdioClient.CallTool(ctx, request)
		}

		a.mcpServer.AddTool(toolCopy, handler)
	}

	return nil
}

// proxyResources 代理资源功能
func (a *StdioToSSEAdapter) proxyResources() error {
	resourcesRequest := mcp.ListResourcesRequest{}
	resourcesResult, err := a.stdioClient.ListResources(a.ctx, resourcesRequest)
	if err != nil {
		// log.Printf("Stdio server does not support resources or failed to list: %v", err)
		return nil
	}

	for _, resource := range resourcesResult.Resources {
		resourceCopy := resource // 捕获循环变量

		handler := func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			// 转发资源读取到 stdio 服务器
			result, err := a.stdioClient.ReadResource(ctx, request)
			if err != nil {
				return nil, err
			}
			return result.Contents, nil
		}

		a.mcpServer.AddResource(resourceCopy, handler)
	}

	return nil
}

// proxyPrompts 代理提示功能
func (a *StdioToSSEAdapter) proxyPrompts() error {
	promptsRequest := mcp.ListPromptsRequest{}
	promptsResult, err := a.stdioClient.ListPrompts(a.ctx, promptsRequest)
	if err != nil {
		//log.Printf("Stdio server does not support prompts or failed to list: %v", err)
		return nil
	}

	// 为每个提示创建代理处理器
	for _, prompt := range promptsResult.Prompts {
		promptCopy := prompt // 捕获循环变量

		handler := func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return a.stdioClient.GetPrompt(ctx, request)
		}

		a.mcpServer.AddPrompt(promptCopy, handler)
	}

	return nil
}

// GetSSEServer 返回 SSE 服务器实例
func (a *StdioToSSEAdapter) GetSSEServer() *server.SSEServer {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.sseServer
}

// GetMCPServer 返回 MCP 服务器实例
func (a *StdioToSSEAdapter) GetMCPServer() *server.MCPServer {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.mcpServer
}

// ServeHTTP 实现 http.Handler 接口
func (a *StdioToSSEAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mutex.RLock()
	sseServer := a.sseServer
	a.mutex.RUnlock()

	if sseServer == nil {
		http.Error(w, "Adapter not initialized", http.StatusInternalServerError)
		return
	}

	sseServer.ServeHTTP(w, r)
}

// StartServer 启动 HTTP 服务器
func (a *StdioToSSEAdapter) StartServer(addr string) error {
	a.mutex.RLock()
	sseServer := a.sseServer
	a.mutex.RUnlock()

	if sseServer == nil {
		return fmt.Errorf("adapter not initialized")
	}

	return sseServer.Start(addr)
}

// Shutdown 优雅关闭适配器
func (a *StdioToSSEAdapter) Shutdown(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running {
		return nil
	}

	var errors []error

	// 关闭 SSE 服务器
	if a.sseServer != nil {
		if err := a.sseServer.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to shutdown SSE server: %w", err))
		}
	}

	// 清理资源
	a.cleanup()

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// cleanup 清理资源
func (a *StdioToSSEAdapter) cleanup() {
	if a.stdioClient != nil {
		a.stdioClient.Close()
		a.stdioClient = nil
	}

	if a.cancel != nil {
		a.cancel()
	}

	a.running = false
}

// IsRunning 检查适配器是否正在运行
func (a *StdioToSSEAdapter) IsRunning() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.running
}

// GetServerInfo 获取原始服务器信息
func (a *StdioToSSEAdapter) GetServerInfo(ctx context.Context) (*mcp.InitializeResult, error) {
	a.mutex.RLock()
	client := a.stdioClient
	a.mutex.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}

	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("stdio server connection lost: %w", err)
	}

	// 返回服务器能力信息
	capabilities := client.GetServerCapabilities()
	return &mcp.InitializeResult{
		Capabilities: capabilities,
	}, nil
}

// RefreshTools 刷新工具列表（重新从 stdio 服务器获取）
func (a *StdioToSSEAdapter) RefreshTools(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running || a.stdioClient == nil {
		return fmt.Errorf("adapter not running")
	}

	return a.proxyTools()
}

// AddNotificationHandler 添加通知处理器
func (a *StdioToSSEAdapter) AddNotificationHandler(handler func(mcp.JSONRPCNotification)) {
	a.mutex.RLock()
	client := a.stdioClient
	a.mutex.RUnlock()

	if client != nil {
		client.OnNotification(handler)
	}
}

// HealthCheck 健康检查
func (a *StdioToSSEAdapter) HealthCheck(ctx context.Context) error {
	a.mutex.RLock()
	client := a.stdioClient
	running := a.running
	a.mutex.RUnlock()

	if !running {
		return fmt.Errorf("adapter not running")
	}

	if client == nil {
		return fmt.Errorf("stdio client not available")
	}

	// 使用 ping 检查连接
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("stdio server ping failed: %w", err)
	}

	return nil
}
