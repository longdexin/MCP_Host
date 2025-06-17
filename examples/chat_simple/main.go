package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Longdexin/MCP_Host"
	"github.com/Longdexin/MCP_Host/llm"
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

func main() {
	// 创建MCP主机
	host := MCP_Host.NewMCPHost()
	defer host.DisconnectAll()

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 连接服务器
	conn1, err := host.ConnectSSE(ctx, "server1", "http://mcp.hm.tianli0.top/mcp/all/"+MCP_API_Secret+"/sse")
	if err != nil {
		log.Fatalf("无法连接到server1: %v", err)
	}
	fmt.Printf("已连接到server1: %s (版本 %s)\n",
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

	// 创建MCP LLM客户端
	mcpClient := llm.NewMCPClient(openaiClient, host)

	fmt.Println("\n--- 文本模式流式示例（不自动执行工具） ---")
	fmt.Print("LLM响应: ")
	textResult, err := mcpClient.Generate(ctx, "现在是几点",
		llm.WithMCPWorkMode(llm.TextMode),
		llm.WithStreamingFunc(streamHandler),
		llm.WithMCPAutoExecute(false),
	)
	if err != nil {
		log.Fatalf("生成失败: %v", err)
	}

	fmt.Println("\n--- 手动执行工具调用 ---")
	tasks, err := mcpClient.ExtractMCPTasks(textResult.Content)
	if err != nil {
		log.Printf("提取任务失败: %v", err)
	} else if len(tasks) > 0 {
		fmt.Printf("发现 %d 个工具调用\n", len(tasks))

		taskResults, err := mcpClient.ExecuteMCPTasksWithResults(ctx, textResult.Content)
		if err != nil {
			log.Printf("执行工具调用失败: %v", err)
		} else {
			fmt.Println("工具调用结果:")
			for i, result := range taskResults {
				fmt.Printf("%d. 任务: %s.%s\n", i+1, result.Task.Server, result.Task.Tool)
				if result.Error != "" {
					fmt.Printf("   错误: %s\n", result.Error)
				} else {
					resultJSON, _ := json.Marshal(result.Result)
					fmt.Printf("   结果: %s\n", string(resultJSON))
				}
			}
		}

		fmt.Println("\n执行工具调用后的更新内容:")
		fmt.Println(textResult.Content)
	} else {
		fmt.Println("未发现工具调用")
	}

	fmt.Println("\n--- 文本模式流式示例（自动执行工具） ---")
	fmt.Print("LLM响应: ")
	autoResult, err := mcpClient.Generate(ctx, "现在是几点",
		llm.WithMCPWorkMode(llm.TextMode),
		llm.WithStreamingFunc(streamHandler),
		llm.WithMCPAutoExecute(true),
	)
	if err != nil {
		log.Fatalf("生成失败: %v", err)
	}

	if autoResult.GenerationInfo != nil {
		if taskResults, ok := autoResult.GenerationInfo["mcp_task_results"].([]llm.TaskResult); ok && len(taskResults) > 0 {
			fmt.Println("\n\n自动执行的工具调用结果:")
			for i, result := range taskResults {
				fmt.Printf("%d. 任务: %s.%s\n", i+1, result.Task.Server, result.Task.Tool)
				if result.Error != "" {
					fmt.Printf("   错误: %s\n", result.Error)
				} else {
					resultJSON, _ := json.Marshal(result.Result)
					fmt.Printf("   结果: %s\n", string(resultJSON))
				}
			}
		}
	}

	fmt.Println("\n--- 函数调用模式流式示例（不自动执行工具） ---")
	fmt.Print("LLM响应: ")
	funcResult, err := mcpClient.Generate(ctx, "现在是几点",
		llm.WithMCPWorkMode(llm.FunctionCallMode),
		llm.WithStreamingFunc(streamHandler),
		llm.WithMCPAutoExecute(false),
	)
	if err != nil {
		log.Fatalf("生成失败: %v", err)
	}

	if len(funcResult.ToolCalls) > 0 {
		fmt.Printf("\n发现 %d 个工具调用，现在执行...\n", len(funcResult.ToolCalls))

		if err := mcpClient.ExecuteToolCalls(ctx, funcResult); err != nil {
			log.Printf("执行工具调用失败: %v", err)
		} else {
			fmt.Println("工具调用结果:")
			for id, result := range funcResult.GenerationInfo {
				if strings.HasPrefix(id, "tool_result_") {
					fmt.Printf("%s: %v\n", id, result)
				}
			}
		}
	}
}
