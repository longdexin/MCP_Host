package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Longdexin/MCP_Host"
	"github.com/mark3labs/mcp-go/mcp"
)

var API_Secret = "XXXXXXXXXXXXXX"

func main() {

	host := MCP_Host.NewMCPHost()
	defer host.DisconnectAll()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host.SetGlobalNotificationHandler(func(serverID string, notification mcp.JSONRPCNotification) {
		fmt.Printf("收到来自服务器 %s 的通知: %s\n", serverID, notification.Method)
	})

	// 连接到SSE服务器
	conn1, err := host.ConnectSSE(ctx, "server1", "http://mcp.hm.tianli0.top/mcp/all/"+API_Secret+"/sse")
	if err != nil {
		log.Fatalf("无法连接到server1: %v", err)
	}
	fmt.Printf("已连接到server1: %s (版本 %s)\n",
		conn1.ServerInfo.ServerInfo.Name,
		conn1.ServerInfo.ServerInfo.Version)

	conn2, err := host.ConnectSSE(ctx, "server2", "http://mcp.hm.tianli0.top/mcp/project/"+API_Secret+"/S-PH8OLSAT69ZZBBHH/sse")
	if err != nil {
		log.Printf("无法连接到server2: %v", err)
	} else {
		fmt.Printf("已连接到server2: %s (版本 %s)\n",
			conn2.ServerInfo.ServerInfo.Name,
			conn2.ServerInfo.ServerInfo.Version)
	}

	tools, err := host.ListTools(ctx, "server1")
	if err != nil {
		log.Printf("无法列出server1的工具: %v", err)
	} else {
		fmt.Printf("Server1有 %d 个工具可用\n", len(tools.Tools))
		for i, tool := range tools.Tools {
			fmt.Printf("  %d. %s - %s\n", i+1, tool.Name, tool.Description)
		}
	}

	tools, err = host.ListTools(ctx, "server2")
	if err != nil {
		log.Printf("无法列出server2的工具: %v", err)
	} else {
		fmt.Printf("Server2有 %d 个工具可用\n", len(tools.Tools))
		for i, tool := range tools.Tools {
			fmt.Printf("  %d. %s - %s\n", i+1, tool.Name, tool.Description)
		}
	}

	result, err := host.ExecuteTool(ctx, "server1", "get_current_time", nil)
	if err != nil {
		log.Printf("执行server1的工具时出错: %v", err)
	} else {
		fmt.Printf("Server1当前时间: %v\n", result.Content)
	}

	if err := host.DisconnectServer("server1"); err != nil {
		log.Printf("断开server1时出错: %v", err)
	} else {
		fmt.Println("已断开server1")
	}

	if err := host.DisconnectServer("server2"); err != nil {
		log.Printf("断开server2时出错: %v", err)
	} else {
		fmt.Println("已断开server2")
	}

	time.Sleep(5 * time.Second)
}
