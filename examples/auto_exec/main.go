package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/TIANLI0/MCP_Host"
	"github.com/TIANLI0/MCP_Host/llm"
)

// 示例配置，实际不可用
const (
	MCP_API_Secret  = "A-AQ3CIGBABPJCIO1UULUQ799CJ03J7VXP"
	OPENAI_API_KEY  = "sk-YV2H91JEFC0FTICEISPIE6HU6XRUASW0"
	OPENAI_MODEL    = "qwen-turbo"
	OPENAI_BASE_URL = "https://api.ai.zhheo.com/v1"
)

// 流式输出回调函数
func streamHandler(ctx context.Context, chunk []byte) error {
	fmt.Print(string(chunk))
	return nil
}

// 状态通知回调函数
func stateNotifyHandler(ctx context.Context, state llm.MCPExecutionState) error {
	switch state.Type {
	case "tool_call":
		fmt.Printf("\n[开始调用工具: %s.%s]\n", state.ServerID, state.ToolName)
	case "tool_result":
		fmt.Printf("\n[工具执行结果: %s.%s.%s]\n", state.ServerID, state.ToolName, state.Data)
	case "generating_response":
		fmt.Printf("\n[生成AI回复...]\n")
	case "process_complete":
		fmt.Printf("\n[处理完成]\n")
	}
	return nil
}

func main() {
	// 创建MCP主机
	host := MCP_Host.NewMCPHost()
	defer host.DisconnectAll()

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 连接服务器
	conn1, err := host.ConnectSSE(ctx, "server1", "http://mcp.hm.tianli0.top/mcp/all/"+MCP_API_Secret+"/sse")
	if err != nil {
		log.Fatalf("无法连接到server1: %v", err)
	}
	fmt.Printf("已连接到server1: %s (版本 %s)\n\n",
		conn1.ServerInfo.ServerInfo.Name,
		conn1.ServerInfo.ServerInfo.Version)

	// 创建OpenAI LLM客户端
	openaiClient, err := llm.NewOpenAIClient(
		llm.WithToken(OPENAI_API_KEY),
		llm.WithOpenAIModel(OPENAI_MODEL),
		llm.WithBaseURL(OPENAI_BASE_URL),
	)
	if err != nil {
		log.Fatalf("无法创建OpenAI客户端: %v", err)
	}

	mcpClient := llm.NewMCPClient(openaiClient, host)

	fmt.Println("--- 文本模式自动工具调用示例 ---")

	// 禁用工具列表
	disabledTools := []string{
		"server1.get_random_inspiration",
	}

	gen, err := mcpClient.Generate(ctx, "现在是几点？然后再来点灵感",
		llm.WithMCPWorkMode(llm.TextMode),
		llm.WithStreamingFunc(streamHandler),
		llm.WithMCPAutoExecute(true),
		llm.WithStateNotifyFunc(stateNotifyHandler),
		llm.WithTemperature(0.7),
		llm.WithMCPDisabledTools(disabledTools),
	)
	if err != nil {
		log.Fatalf("生成失败: %v", err)
	}

	fmt.Println("\n\n\n生成的文本:\n" + gen.Content)

}
