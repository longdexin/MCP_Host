# MCP_Host

MCP_Host 用于管理和简化与多个 (MCP) 服务器的通信。它提供了连接管理、工具调用和大语言模型集成等功能。

## 特性

- **多服务器连接管理**：统一管理多个 MCP 服务器连接
- **多种连接方式**：支持 SSE、Stdio 和进程内连接方式
- **大语言模型集成**：内置与 OpenAI 等 LLM 的集成
- **工具调用管理**：简化工具调用流程，支持自动工具执行
- **灵活的通知系统**：支持服务器通知的处理和转发

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

## 与大语言模型集成

MCP_Host 内置了与 LLM 的集成，支持文本模式和函数调用模式的工具使用：

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
        }
        return nil
    }),
)
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

## 示例

完整示例可以在 `examples` 目录中找到：

- `examples/simple/main.go` - 基本连接示例
- `examples/chat_simple/main.go` - 与 LLM 集成的基本示例
- `examples/auto_exec/main.go` - 自动执行工具的高级示例

## 许可证

MIT License