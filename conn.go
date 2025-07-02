package MCP_Host

import (
	"context"
	"fmt"
	"sync"

	"maps"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ConnectionType string

const (
	SSEConnectionType       ConnectionType = "SSE"
	StdioConnectionType     ConnectionType = "Stdio"
	InProcessConnectionType ConnectionType = "InProcess"
)

// ServerConnection  到单个MCP服务器的连接
type ServerConnection struct {
	Type         ConnectionType
	Client       *client.Client
	ServerID     string
	Options      []transport.ClientOption
	BaseURL      string
	ServerInfo   *mcp.InitializeResult
	Capabilities mcp.ServerCapabilities
	Connected    bool
}

// MCPHost 管理多个MCP服务器连接
type MCPHost struct {
	connections map[string]*ServerConnection
	mutex       sync.RWMutex
}

// NewMCPHost 创建一个新的MCP Host实例
func NewMCPHost() *MCPHost {
	return &MCPHost{
		connections: make(map[string]*ServerConnection),
	}
}

// ConnectSSE 使用SSE传输连接到MCP服务器
func (h *MCPHost) ConnectSSE(ctx context.Context, serverID string, baseURL string, options ...transport.ClientOption) (*ServerConnection, error) {
	h.mutex.RLock()
	_, exists := h.connections[serverID]
	h.mutex.RUnlock()
	if exists {
		return nil, fmt.Errorf("connection with ID %s already exists", serverID)
	}

	c, err := client.NewSSEMCPClient(baseURL, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE client: %w", err)
	}

	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to start client: %w", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "MCP Host",
		Version: "1.0.0",
	}

	serverInfo, err := c.Initialize(ctx, initRequest)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to initialize connection: %w", err)
	}

	conn := &ServerConnection{
		Type:         SSEConnectionType,
		Client:       c,
		ServerID:     serverID,
		Options:      options,
		BaseURL:      baseURL,
		ServerInfo:   serverInfo,
		Capabilities: serverInfo.Capabilities,
		Connected:    true,
	}

	// 将连接添加到映射
	h.mutex.Lock()
	h.connections[serverID] = conn
	h.mutex.Unlock()

	return conn, nil
}

// ConnectStdio 使用Stdio传输连接到MCP服务器
func (h *MCPHost) ConnectStdio(ctx context.Context, serverID string, command string, env []string, args ...string) (*ServerConnection, error) {
	h.mutex.RLock()
	_, exists := h.connections[serverID]
	h.mutex.RUnlock()
	if exists {
		return nil, fmt.Errorf("connection with ID %s already exists", serverID)
	}

	c, err := client.NewStdioMCPClient(command, env, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdio client: %w", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "MCP Host",
		Version: "1.0.0",
	}

	serverInfo, err := c.Initialize(ctx, initRequest)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to initialize connection: %w", err)
	}

	conn := &ServerConnection{
		Type:         StdioConnectionType,
		Client:       c,
		ServerID:     serverID,
		ServerInfo:   serverInfo,
		Capabilities: serverInfo.Capabilities,
		Connected:    true,
	}

	h.mutex.Lock()
	h.connections[serverID] = conn
	h.mutex.Unlock()

	return conn, nil
}

// ConnectInProcess 使用进程内传输方式连接到MCP服务器
func (h *MCPHost) ConnectInProcess(ctx context.Context, serverID string, server *server.MCPServer) (*ServerConnection, error) {
	h.mutex.RLock()
	_, exists := h.connections[serverID]
	h.mutex.RUnlock()
	if exists {
		return nil, fmt.Errorf("connection with ID %s already exists", serverID)
	}

	c, err := client.NewInProcessClient(server)
	if err != nil {
		return nil, fmt.Errorf("failed to create in-process client: %w", err)
	}

	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to start client: %w", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "MCP Host",
		Version: "1.0.0",
	}

	serverInfo, err := c.Initialize(ctx, initRequest)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to initialize connection: %w", err)
	}

	conn := &ServerConnection{
		Type:         InProcessConnectionType,
		Client:       c,
		ServerID:     serverID,
		ServerInfo:   serverInfo,
		Capabilities: serverInfo.Capabilities,
		Connected:    true,
	}

	h.mutex.Lock()
	h.connections[serverID] = conn
	h.mutex.Unlock()

	return conn, nil
}

// GetConnection 通过ID获取服务器连接
func (h *MCPHost) GetConnection(serverID string) (*ServerConnection, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	conn, exists := h.connections[serverID]
	return conn, exists
}

// GetAllConnections 返回所有服务器连接的映射副本
func (h *MCPHost) GetAllConnections() map[string]*ServerConnection {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	connections := make(map[string]*ServerConnection, len(h.connections))
	maps.Copy(connections, h.connections)

	return connections
}

// DisconnectServer 关闭到指定服务器的连接并将其从映射中移除
func (h *MCPHost) DisconnectServer(serverID string) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	conn, exists := h.connections[serverID]
	if !exists {
		return fmt.Errorf("no connection found with ID %s", serverID)
	}

	// 关闭连接
	err := conn.Client.Close()

	delete(h.connections, serverID)
	conn.Connected = false

	return err
}

// DisconnectAll 关闭所有连接
func (h *MCPHost) DisconnectAll() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for id, conn := range h.connections {
		conn.Client.Close()
		conn.Connected = false
		delete(h.connections, id)
	}
}

// SetNotificationHandler 为特定服务器设置通知处理程序
func (h *MCPHost) SetNotificationHandler(serverID string, handler func(mcp.JSONRPCNotification)) error {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	conn, exists := h.connections[serverID]
	if !exists {
		return fmt.Errorf("no connection found with ID %s", serverID)
	}

	conn.Client.OnNotification(handler)
	return nil
}

// SetGlobalNotificationHandler 为所有服务器设置通知处理程序
func (h *MCPHost) SetGlobalNotificationHandler(handler func(serverID string, notification mcp.JSONRPCNotification)) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for id, conn := range h.connections {
		// 为每个连接捕获serverID
		serverID := id
		conn.Client.OnNotification(func(notification mcp.JSONRPCNotification) {
			handler(serverID, notification)
		})
	}
}

func (h *MCPHost) EnsureConnection(ctx context.Context, serverID string) (*ServerConnection, error) {
	h.mutex.RLock()
	conn, exists := h.connections[serverID]
	h.mutex.RUnlock()
	if !exists {
		return nil, fmt.Errorf("no connection found with ID %s", serverID)
	}
	err := conn.Client.Ping(ctx)
	if err != nil {
		h.DisconnectServer(serverID)
		switch conn.Type {
		case SSEConnectionType:
			conn, err = h.ConnectSSE(ctx, conn.ServerID, conn.BaseURL, conn.Options...)
			if err != nil {
				return nil, fmt.Errorf("can not reconnect with ID %s", serverID)
			}
		default:
		}
	}
	return conn, nil
}

// ExecuteTool 在指定服务器上执行工具
func (h *MCPHost) ExecuteTool(ctx context.Context, serverID string, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	conn, err := h.EnsureConnection(ctx, serverID)
	if err != nil {
		return nil, err
	}
	request := mcp.CallToolRequest{}
	request.Params.Name = toolName
	request.Params.Arguments = args
	return conn.Client.CallTool(ctx, request)
}

// ListTools 列出指定服务器上的所有工具
func (h *MCPHost) ListTools(ctx context.Context, serverID string) (*mcp.ListToolsResult, error) {
	conn, err := h.EnsureConnection(ctx, serverID)
	if err != nil {
		return nil, err
	}
	request := mcp.ListToolsRequest{}
	return conn.Client.ListTools(ctx, request)
}

// ListResources 列出指定服务器上的所有资源
func (h *MCPHost) ListResources(ctx context.Context, serverID string) (*mcp.ListResourcesResult, error) {
	conn, err := h.EnsureConnection(ctx, serverID)
	if err != nil {
		return nil, err
	}
	request := mcp.ListResourcesRequest{}
	return conn.Client.ListResources(ctx, request)
}

// ReadResource 从指定服务器读取资源
func (h *MCPHost) ReadResource(ctx context.Context, serverID string, uri string) (*mcp.ReadResourceResult, error) {
	conn, err := h.EnsureConnection(ctx, serverID)
	if err != nil {
		return nil, err
	}
	request := mcp.ReadResourceRequest{}
	request.Params.URI = uri
	return conn.Client.ReadResource(ctx, request)
}
