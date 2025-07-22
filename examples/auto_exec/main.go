package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/longdexin/MCP_Host"
	"github.com/longdexin/MCP_Host/llm"
)

// 示例配置，实际不可用
const (
	MCP_API_Secret  = "A-AQ3CIGBABPJCIO1UULUQ799CJ03J7VXP"
	OPENAI_API_KEY  = "sk-YV2H91JEFC0FTICEISPIE6HU6XRUASW0"
	OPENAI_MODEL    = "qwen-turbo"
	OPENAI_BASE_URL = "https://api.ai.zhheo.com/v1"
)

// 流式输出回调函数
func streamHandler(ctx context.Context, chunk []byte, toolResults []llm.MCPToolExecutionResult) error {
	fmt.Print(string(chunk))
	return nil
}

// 状态通知回调函数
func stateNotifyHandler(ctx context.Context, state llm.MCPExecutionState) error {
	switch state.Type {
	case "tool_call":
		fmt.Printf("\n[开始调用工具: %s.%s]\n", state.ServerID, state.ToolName)
	case "tool_result":
		fmt.Printf("\n[工具执行结果: %s.%s]\n", state.ServerID, state.ToolName)
	case "generating_response":
		fmt.Printf("\n[生成AI回复...]\n")
	case "process_complete":
		if rounds, ok := state.Data["execution_rounds"].(int); ok {
			fmt.Printf("\n[处理完成, 共执行 %d 轮工具调用]\n", rounds)
		} else {
			fmt.Printf("\n[处理完成]\n")
		}
	case "execution_round":
		if round, ok := state.Data["round"].(int); ok {
			fmt.Printf("\n[开始执行第 %d 轮工具调用]\n", round)
		}
	case "intermediate_generation":
		if state.Stage == "start" {
			fmt.Printf("\n[生成中间分析...]\n")
		} else if state.Stage == "complete" {
			fmt.Printf("\n[中间分析生成完成]\n")
		}
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
	conn1, err := host.ConnectSSE(ctx, "server1", "https://mcp.amap.com/sse?key="+MCP_API_Secret)
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

	_, err = mcpClient.Generate(ctx, "从苏州站到上海虹桥站的最佳出行方案是什么？",
		llm.WithMCPWorkMode(llm.TextMode),
		llm.WithStreamingFunc(streamHandler),
		llm.WithMCPAutoExecute(true),
		llm.WithStateNotifyFunc(stateNotifyHandler),
		llm.WithMCPDisabledTools(disabledTools),
		llm.WithMCPMaxToolExecutionRounds(5),
	)
	if err != nil {
		log.Fatalf("生成失败: %v", err)
	}

}
