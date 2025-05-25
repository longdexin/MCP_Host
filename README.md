# MCP_Host

MCP_Host 是一个用于管理和简化与多个 MCP (Model Context Protocol) 服务器通信的 Go 语言库。它提供了连接管理、工具调用、大语言模型集成和协议适配等功能。

## TODO

- [ ] 完善文档
- [x] 支持调用Tools
- [ ] 支持资源查找
- [x] stdio到SSE协议适配器

## 特性

- **多服务器连接管理**：统一管理多个 MCP 服务器连接
- **多种连接方式**：支持 SSE、Stdio 和进程内连接方式
- **大语言模型集成**：内置与 OpenAI 等 LLM 的集成，支持文本模式和函数调用模式
- **工具调用管理**：简化工具调用流程，支持自动工具执行和多轮工具调用
- **灵活的通知系统**：支持服务器通知的处理和转发
- **协议适配**：提供 stdio 到 SSE 的协议适配器
- **流式响应**：支持流式输出和实时状态通知
- **资源管理**：支持 MCP 资源的列表和读取

## 安装

```go
go get github.com/TIANLI0/MCP_Host
```

## 基本用法

### 连接 MCP 服务器

```go
import (
    "context"
    "fmt"
    "time"
    
    "github.com/TIANLI0/MCP_Host"
)

func main() {
    host := MCP_Host.NewMCPHost()
    defer host.DisconnectAll()
    
    ctx := context.Background()
    
    // 连接到 SSE 服务器
    conn, err := host.ConnectSSE(ctx, "server1", "http://your-mcp-server-url/sse")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("已连接到: %s (版本 %s)\n", 
        conn.ServerInfo.ServerInfo.Name, 
        conn.ServerInfo.ServerInfo.Version)
}
```

### 执行工具调用

```go
func executeTools(host *MCP_Host.MCPHost, ctx context.Context) {
    // 列出可用工具
    tools, err := host.ListTools(ctx, "server1")
    if err != nil {
        fmt.Printf("无法列出工具: %v\n", err)
        return
    }
    
    fmt.Printf("发现 %d 个可用工具\n", len(tools.Tools))
    
    // 执行工具
    result, err := host.ExecuteTool(ctx, "server1", "get_current_time", nil)
    if err != nil {
        fmt.Printf("工具执行失败: %v\n", err)
        return
    }
    
    fmt.Printf("工具执行结果: %v\n", result.Content)
}
```

### 资源管理

```go
func manageResources(host *MCP_Host.MCPHost, ctx context.Context) {
    // 列出可用资源
    resources, err := host.ListResources(ctx, "server1")
    if err != nil {
        fmt.Printf("无法列出资源: %v\n", err)
        return
    }
    
    fmt.Printf("发现 %d 个可用资源\n", len(resources.Resources))
    
    // 读取特定资源
    if len(resources.Resources) > 0 {
        resource, err := host.ReadResource(ctx, "server1", resources.Resources[0].URI)
        if err != nil {
            fmt.Printf("读取资源失败: %v\n", err)
            return
        }
        
        fmt.Printf("资源内容: %v\n", resource.Contents)
    }
}
```

## 与大语言模型集成

MCP_Host 内置了与 LLM 的集成，支持文本模式和函数调用模式的工具使用：

### 基本使用

```go
import (
    "github.com/TIANLI0/MCP_Host"
    "github.com/TIANLI0/MCP_Host/llm"
)

func useLLM(host *MCP_Host.MCPHost, ctx context.Context) {
    // 创建 OpenAI 客户端
    openaiClient, err := llm.NewOpenAIClient(
        llm.WithToken("your-api-key"),
        llm.WithOpenAIModel("gpt-4"),
        llm.WithBaseURL("https://api.openai.com/v1"),
    )
    if err != nil {
        panic(err)
    }
    
    // 创建 MCP 客户端包装
    mcpClient := llm.NewMCPClient(openaiClient, host)
    
    // 自动执行工具调用
    gen, err := mcpClient.Generate(ctx, "现在是几点？",
        llm.WithMCPWorkMode(llm.TextMode),
        llm.WithMCPAutoExecute(true),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Println(gen.Content)
}
```

### 工作模式

#### 文本模式 (TextMode)
在文本模式下，LLM 会在响应中生成特定格式的工具调用标签：

```go
gen, err := mcpClient.Generate(ctx, "查询天气信息",
    llm.WithMCPWorkMode(llm.TextMode),
    llm.WithMCPAutoExecute(true),
    llm.WithMCPTaskTag("MCP_HOST_TASK"),
    llm.WithMCPResultTag("MCP_HOST_RESULT"),
)
```

#### 函数调用模式 (FunctionCallMode)
在函数调用模式下，使用标准的 OpenAI 函数调用格式：

```go
gen, err := mcpClient.Generate(ctx, "查询天气信息",
    llm.WithMCPWorkMode(llm.FunctionCallMode),
    llm.WithMCPAutoExecute(true),
)
```

### 多轮工具执行

```go
gen, err := mcpClient.Generate(ctx, "帮我规划从北京到上海的出行方案",
    llm.WithMCPWorkMode(llm.TextMode),
    llm.WithMCPAutoExecute(true),
    llm.WithMCPMaxToolExecutionRounds(5), // 最多执行5轮工具调用
)
```

## 高级功能

### 流式输出

```go
// 设置流式输出回调
gen, err := mcpClient.Generate(ctx, "需要执行的任务",
    llm.WithMCPAutoExecute(true),
    llm.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
        fmt.Print(string(chunk))
        return nil
    }),
)
```

### 状态通知

```go
gen, err := mcpClient.Generate(ctx, "复杂任务",
    llm.WithMCPAutoExecute(true),
    llm.WithStateNotifyFunc(func(ctx context.Context, state llm.MCPExecutionState) error {
        switch state.Type {
        case "tool_call":
            fmt.Printf("\n[调用工具: %s.%s]\n", state.ServerID, state.ToolName)
        case "tool_result":
            fmt.Printf("\n[工具结果]\n")
        case "execution_round":
            if round, ok := state.Data["round"].(int); ok {
                fmt.Printf("\n[开始第 %d 轮执行]\n", round)
            }
        case "process_complete":
            fmt.Printf("\n[处理完成]\n")
        }
        return nil
    }),
)
```

### 禁用特定工具

```go
// 禁用特定工具
disabledTools := []string{
    "server1.dangerous_tool",
    "server2.slow_tool",
}

gen, err := mcpClient.Generate(ctx, "执行安全的任务",
    llm.WithMCPDisabledTools(disabledTools),
)
```

### 手动工具执行

```go
// 不自动执行，手动控制工具调用
gen, err := mcpClient.Generate(ctx, "现在是几点",
    llm.WithMCPWorkMode(llm.TextMode),
    llm.WithMCPAutoExecute(false),
)

// 提取工具调用任务
tasks, err := mcpClient.ExtractMCPTasks(gen.Content)
if err == nil && len(tasks) > 0 {
    // 手动执行工具调用
    results, err := mcpClient.ExecuteMCPTasksWithResults(ctx, gen.Content)
    if err != nil {
        log.Printf("执行失败: %v", err)
    }
    
    for _, result := range results {
        fmt.Printf("工具 %s.%s 结果: %v\n", 
            result.Task.Server, result.Task.Tool, result.Result)
    }
}
```

## 自定义 MCP 服务器连接

除了 SSE 连接外，MCP_Host 还支持其他连接方式：

### 标准输入输出连接

```go
conn, err := host.ConnectStdio(ctx, "local-server", "./mcp-server", 
    []string{"ENV=production"}, "--debug")
```

### 进程内连接

```go
import "github.com/mark3labs/mcp-go/server"

// 创建服务器实例
server := server.NewMCPServer()
// 添加工具
server.RegisterTool("get_time", func(ctx context.Context, args map[string]any) (any, error) {
    return time.Now().String(), nil
})

// 连接
conn, err := host.ConnectInProcess(ctx, "embedded", server)
```

## Stdio 到 SSE 适配器

MCP_Host 提供了一个适配器，可以将使用 stdio 协议的 MCP 服务器转换为 SSE 服务器：

### 使用适配器

```go
import "github.com/TIANLI0/MCP_Host/adapters/stdio2sse"

// 创建适配器
adapter := stdio2sse.NewStdioToSSEAdapter(
    "python", // 命令
    []string{"mcp_server.py"}, // 参数
    stdio2sse.WithEnvironment([]string{"PATH=" + os.Getenv("PATH")}),
)

// 初始化适配器
if err := adapter.Initialize(); err != nil {
    log.Fatalf("初始化失败: %v", err)
}

// 创建 HTTP 服务器
mux := http.NewServeMux()
mux.Handle("/sse", adapter.GetSSEServer().SSEHandler())
mux.Handle("/message", adapter.GetSSEServer().MessageHandler())

// 健康检查
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    if err := adapter.HealthCheck(r.Context()); err != nil {
        http.Error(w, err.Error(), http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
})

server := &http.Server{Addr: ":8080", Handler: mux}
server.ListenAndServe()
```

### 命令行工具

你也可以直接使用命令行工具来启动适配器：

```bash
go run examples/stdio2sse/main.go python mcp_server.py

# 服务器将在 :8057 端口启动
# SSE 端点: http://localhost:8057/sse
# 健康检查: http://localhost:8057/health
```

## API 参考

### MCPHost 方法

```go
// 连接管理
func (h *MCPHost) ConnectSSE(ctx context.Context, serverID, url string) (*Connection, error)
func (h *MCPHost) ConnectStdio(ctx context.Context, serverID, command string, env []string, args ...string) (*Connection, error)
func (h *MCPHost) DisconnectServer(serverID string) error
func (h *MCPHost) DisconnectAll()

// 工具操作
func (h *MCPHost) ListTools(ctx context.Context, serverID string) (*mcp.ListToolsResult, error)
func (h *MCPHost) ExecuteTool(ctx context.Context, serverID, toolName string, args map[string]any) (*mcp.CallToolResult, error)

// 资源操作
func (h *MCPHost) ListResources(ctx context.Context, serverID string) (*mcp.ListResourcesResult, error)
func (h *MCPHost) ReadResource(ctx context.Context, serverID, uri string) (*mcp.ReadResourceResult, error)

// 通知处理
func (h *MCPHost) SetNotificationHandler(serverID string, handler func(mcp.JSONRPCNotification)) error
func (h *MCPHost) SetGlobalNotificationHandler(handler func(serverID string, notification mcp.JSONRPCNotification))
```

### MCPClient 选项

```go
// 工作模式
llm.WithMCPWorkMode(llm.TextMode)           // 文本模式
llm.WithMCPWorkMode(llm.FunctionCallMode)   // 函数调用模式

// 执行控制
llm.WithMCPAutoExecute(true)                // 自动执行工具调用
llm.WithMCPMaxToolExecutionRounds(5)        // 最大执行轮次
llm.WithMCPDisabledTools([]string{"server.tool"}) // 禁用工具

// 流式和通知
llm.WithStreamingFunc(func(ctx context.Context, chunk []byte) error { ... })
llm.WithStateNotifyFunc(func(ctx context.Context, state llm.MCPExecutionState) error { ... })

// 标签自定义
llm.WithMCPTaskTag("CUSTOM_TASK")          // 自定义任务标签
llm.WithMCPResultTag("CUSTOM_RESULT")      // 自定义结果标签
llm.WithMCPPrompt("custom prompt...")       // 自定义提示词
```

## 示例

完整示例可以在 `examples` 目录中找到：

- [`examples/simple/main.go`](examples/simple/main.go) - 基本连接和工具调用示例
- [`examples/chat_simple/main.go`](examples/chat_simple/main.go) - 与 LLM 集成的基本示例
- [`examples/auto_exec/main.go`](examples/auto_exec/main.go) - 自动执行工具的高级示例，包含状态通知
- [`examples/stdio2sse/main.go`](examples/stdio2sse/main.go) - Stdio 到 SSE 适配器示例

## 许可证

MIT License