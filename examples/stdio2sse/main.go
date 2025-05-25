package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TIANLI0/MCP_Host/adapters/stdio2sse"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {

	command := os.Args[1]
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}

	// 创建适配器
	adapter := stdio2sse.NewStdioToSSEAdapter(
		command,
		args,
		stdio2sse.WithEnvironment([]string{"PATH=" + os.Getenv("PATH")}),
	)

	// 初始化适配器
	log.Printf("Initializing adapter for command: %s %v", command, args)
	if err := adapter.Initialize(); err != nil {
		log.Fatalf("Failed to initialize adapter: %v", err)
	}

	// 设置通知处理器
	adapter.AddNotificationHandler(func(notification mcp.JSONRPCNotification) {
		log.Printf("Received notification: %s", notification.Method)
	})

	// 创建 HTTP 服务器
	mux := http.NewServeMux()

	// 添加适配器处理器
	mux.Handle("/sse", adapter.GetSSEServer().SSEHandler())
	mux.Handle("/message", adapter.GetSSEServer().MessageHandler())

	// 添加健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := adapter.HealthCheck(ctx); err != nil {
			http.Error(w, fmt.Sprintf("Health check failed: %v", err), http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
	})

	// 添加服务器信息端点
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		info, err := adapter.GetServerInfo(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get server info: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, `{
            "status":"running",
            "server_capabilities":{
                "tools":%t,
                "resources":%t,
                "prompts":%t
            },
            "timestamp":"%s"
        }`,
			info.Capabilities.Tools != nil,
			info.Capabilities.Resources != nil,
			info.Capabilities.Prompts != nil,
			time.Now().Format(time.RFC3339),
		)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Stdio2SSE Adapter</title>
</head>
<body>
    <h1>Stdio2SSE Adapter</h1>
    <p>This server proxies a stdio MCP server as an SSE server.</p>
    <ul>
        <li><a href="/sse">SSE Endpoint</a></li>
        <li><a href="/health">Health Check</a></li>
        <li><a href="/info">Server Info</a></li>
    </ul>
    <p>To connect a client, use the SSE endpoint: <code>%s/sse</code></p>
</body>
</html>`, r.Host)
	})

	server := &http.Server{
		Addr:    ":8057",
		Handler: mux,
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := adapter.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down adapter: %v", err)
		}

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}()

	log.Printf("Starting server on :8057")
	log.Printf("SSE endpoint: http://localhost:8057/sse")
	log.Printf("Health check: http://localhost:8057/health")
	log.Printf("Server info: http://localhost:8057/info")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}
