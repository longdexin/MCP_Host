package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/TIANLI0/MCP_Host"
)

// MCPTask MCP任务
type MCPTask struct {
	Server string         `json:"server"` // 服务器ID
	Tool   string         `json:"tool"`   // 工具名称
	Args   map[string]any `json:"args"`   // 参数
}

// TaskResult 任务执行的结果
type TaskResult struct {
	Task   MCPTask `json:"task"`            // 执行的任务
	Result any     `json:"result"`          // 执行结果
	Error  string  `json:"error,omitempty"` // 错误信息，如果有的话
}

// MCPClient MCP的LLM客户端包装
type MCPClient struct {
	llm    LLM               // 底层LLM客户端
	host   *MCP_Host.MCPHost // MCP主机
	prompt string            // 默认提示
}

// NewMCPClient 创建一个新的MCPClient
func NewMCPClient(llm LLM, host *MCP_Host.MCPHost) *MCPClient {
	return &MCPClient{
		llm:    llm,
		host:   host,
		prompt: defaultMCPPrompt,
	}
}

// SetPrompt 设置默认提示
func (c *MCPClient) SetPrompt(prompt string) {
	c.prompt = prompt
}

// Generate 生成回复并处理MCP任务
func (c *MCPClient) Generate(ctx context.Context, prompt string, options ...GenerateOption) (*Generation, error) {
	opts := DefaultGenerateOption()
	for _, opt := range options {
		opt(opts)
	}

	// 在文本模式下添加MCP提示
	if opts.MCPWorkMode == TextMode {
		mcpPrompt := opts.MCPPrompt
		if mcpPrompt == "" {
			mcpPrompt = c.prompt
		}
		toolsInfo := c.formatMCPToolsAsText(ctx, opts.MCPTaskTag)

		systemPrompt := mcpPrompt
		if toolsInfo != "" {
			systemPrompt += "\n\n" + toolsInfo
		}

		messages := []Message{
			*NewSystemMessage("", systemPrompt),
			*NewUserMessage("", prompt),
		}

		gen, err := c.llm.GenerateContent(ctx, messages, options...)
		if err != nil {
			return nil, err
		}

		if opts.MCPAutoExecute {
			taskResults, err := c.processMCPTasksWithResults(ctx, gen, opts.MCPTaskTag)
			if err != nil {
				return nil, err
			}

			if len(taskResults) > 0 {
				if gen.GenerationInfo == nil {
					gen.GenerationInfo = make(map[string]any)
				}
				gen.GenerationInfo["mcp_task_results"] = taskResults
			}
		}

		return gen, nil
	} else {
		// 函数调用模式
		tools := c.createMCPTools(ctx)

		toolsOption := WithTools(tools)
		gen, err := c.llm.Generate(ctx, prompt, append(options, toolsOption)...)
		if err != nil {
			return nil, err
		}

		if opts.MCPAutoExecute {
			if err := c.processToolCalls(ctx, gen); err != nil {
				return nil, err
			}
		} else {
			c.appendToolCallsToContent(gen, opts.MCPTaskTag)
		}

		return gen, nil
	}
}

// GenerateContent 生成回复并处理MCP任务
func (c *MCPClient) GenerateContent(ctx context.Context, messages []Message, options ...GenerateOption) (*Generation, error) {
	opts := DefaultGenerateOption()
	for _, opt := range options {
		opt(opts)
	}

	// 在文本模式下添加MCP提示
	if opts.MCPWorkMode == TextMode {
		mcpPrompt := opts.MCPPrompt
		if mcpPrompt == "" {
			mcpPrompt = c.prompt
		}

		// 获取工具信息
		toolsInfo := c.formatMCPToolsAsText(ctx, opts.MCPTaskTag)

		systemPrompt := mcpPrompt
		if toolsInfo != "" {
			systemPrompt += "\n\n" + toolsInfo
		}

		// 添加系统提示
		hasSystemMsg := false
		var allMessages []Message

		for _, msg := range messages {
			if msg.Role == RoleSystem {
				hasSystemMsg = true
				newMsg := msg
				if toolsInfo != "" {
					newMsg.Content = msg.Content + "\n\n" + toolsInfo
				}
				allMessages = append(allMessages, newMsg)
			} else {
				allMessages = append(allMessages, msg)
			}
		}

		if !hasSystemMsg && systemPrompt != "" {
			allMessages = append([]Message{*NewSystemMessage("", systemPrompt)}, allMessages...)
		}

		gen, err := c.llm.GenerateContent(ctx, allMessages, options...)
		if err != nil {
			return nil, err
		}

		if opts.MCPAutoExecute {
			taskResults, err := c.processMCPTasksWithResults(ctx, gen, opts.MCPTaskTag)
			if err != nil {
				return nil, err
			}

			if len(taskResults) > 0 {
				if gen.GenerationInfo == nil {
					gen.GenerationInfo = make(map[string]any)
				}
				gen.GenerationInfo["mcp_task_results"] = taskResults
			}
		}

		return gen, nil
	} else {
		// 函数调用模式
		tools := c.createMCPTools(ctx)
		toolsOption := WithTools(tools)

		gen, err := c.llm.GenerateContent(ctx, messages, append(options, toolsOption)...)
		if err != nil {
			return nil, err
		}

		if opts.MCPAutoExecute {
			if err := c.processToolCalls(ctx, gen); err != nil {
				return nil, err
			}
		} else {
			c.appendToolCallsToContent(gen, opts.MCPTaskTag)
		}

		return gen, nil
	}
}

// processMCPTasksWithResults 解析并执行任务，返回结果列表
func (c *MCPClient) processMCPTasksWithResults(ctx context.Context, gen *Generation, taskTag string) ([]TaskResult, error) {
	tasks, err := c.ExtractMCPTasks(gen.Content, taskTag)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, nil
	}

	var results []TaskResult

	// 执行任务
	for _, task := range tasks {
		taskResult := TaskResult{
			Task: task,
		}

		result, err := c.host.ExecuteTool(ctx, task.Server, task.Tool, task.Args)
		if err != nil {
			taskResult.Error = err.Error()
		} else {
			taskResult.Result = result.Content
		}

		results = append(results, taskResult)
	}

	return results, nil
}

// ExecuteMCPTasksWithResults 执行文本中提取的MCP任务并返回结果列表
func (c *MCPClient) ExecuteMCPTasksWithResults(ctx context.Context, content string, taskTag string) ([]TaskResult, error) {
	// 提取MCP任务
	tasks, err := c.ExtractMCPTasks(content, taskTag)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, nil
	}

	var results []TaskResult

	// 执行任务
	for _, task := range tasks {
		taskResult := TaskResult{
			Task: task,
		}

		result, err := c.host.ExecuteTool(ctx, task.Server, task.Tool, task.Args)
		if err != nil {
			taskResult.Error = err.Error()
		} else {
			taskResult.Result = result.Content
		}

		results = append(results, taskResult)
	}

	return results, nil
}

// ExecuteMCPTasks 执行文本中提取的MCP任务 (保留以兼容现有代码)
func (c *MCPClient) ExecuteMCPTasks(ctx context.Context, content string, taskTag string) (string, error) {
	// 提取MCP任务
	tasks, err := c.ExtractMCPTasks(content, taskTag)
	if err != nil {
		return content, err
	}

	if len(tasks) == 0 {
		return content, nil
	}

	// 执行任务并替换结果
	updatedContent := content
	for _, task := range tasks {
		result, err := c.host.ExecuteTool(ctx, task.Server, task.Tool, task.Args)
		if err != nil {
			updatedContent = strings.Replace(
				updatedContent,
				fmt.Sprintf("<%s>\n%s\n</%s>", taskTag, taskToString(task), taskTag),
				fmt.Sprintf("<%s>\n%s\n[ERROR] %v\n</%s>", taskTag, taskToString(task), err, taskTag),
				1,
			)
			continue
		}

		resultStr, _ := json.Marshal(result.Content)
		updatedContent = strings.Replace(
			updatedContent,
			fmt.Sprintf("<%s>\n%s\n</%s>", taskTag, taskToString(task), taskTag),
			fmt.Sprintf("<%s>\n%s\n[RESULT] %s\n</%s>", taskTag, taskToString(task), string(resultStr), taskTag),
			1,
		)
	}

	return updatedContent, nil
}

// appendToolCallsToContent 将工具调用示例添加到内容中
func (c *MCPClient) appendToolCallsToContent(gen *Generation, taskTag string) {
	if len(gen.ToolCalls) == 0 {
		return
	}

	var toolCallsText strings.Builder
	toolCallsText.WriteString("\n\n")

	for _, call := range gen.ToolCalls {
		parts := strings.Split(call.Function.Name, ".")
		if len(parts) != 2 {
			continue
		}

		serverID := parts[0]
		toolName := parts[1]

		task := MCPTask{
			Server: serverID,
			Tool:   toolName,
		}

		if err := json.Unmarshal([]byte(call.Function.Arguments), &task.Args); err != nil {
			continue
		}

		toolCallsText.WriteString(fmt.Sprintf("<%s>\n%s\n</%s>\n", taskTag, taskToString(task), taskTag))
	}

	gen.Content += toolCallsText.String()
}

// ExtractMCPTasks 从文本中提取MCP任务
func (c *MCPClient) ExtractMCPTasks(content string, taskTag string) ([]MCPTask, error) {
	return extractMCPTasks(content, taskTag)
}

// ExecuteToolCalls 执行工具调用并返回更新后的生成结果
func (c *MCPClient) ExecuteToolCalls(ctx context.Context, gen *Generation) error {
	if len(gen.ToolCalls) == 0 {
		return nil
	}

	if gen.GenerationInfo == nil {
		gen.GenerationInfo = make(map[string]any)
	}

	for _, call := range gen.ToolCalls {
		parts := strings.Split(call.Function.Name, ".")
		if len(parts) != 2 {
			continue
		}

		serverID := parts[0]
		toolName := parts[1]

		// 解析参数
		var args map[string]any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			continue
		}

		// 执行工具
		result, err := c.host.ExecuteTool(ctx, serverID, toolName, args)
		if err != nil {
			gen.GenerationInfo["tool_error_"+call.ID] = err.Error()
			continue
		}

		gen.GenerationInfo["tool_result_"+call.ID] = result.Content
	}

	return nil
}

// formatMCPToolsAsText 将MCP工具信息格式化为文本形式
func (c *MCPClient) formatMCPToolsAsText(ctx context.Context, taskTag string) string {
	var builder strings.Builder
	builder.WriteString("可用工具列表:\n\n")

	connections := c.host.GetAllConnections()

	hasTools := false

	// 遍历每个连接
	for serverID := range connections {
		toolsResult, err := c.host.ListTools(ctx, serverID)
		if err != nil {
			continue
		}

		if len(toolsResult.Tools) > 0 {
			hasTools = true
			builder.WriteString(fmt.Sprintf("服务器 %s:\n", serverID))

			for _, tool := range toolsResult.Tools {
				builder.WriteString(fmt.Sprintf("  - %s: %s\n", tool.Name, tool.Description))

				var properties map[string]any

				inputSchemaMap := make(map[string]any)
				schemaBytes, err := json.Marshal(tool.InputSchema)
				if err == nil {
					_ = json.Unmarshal(schemaBytes, &inputSchemaMap)

					if props, ok := inputSchemaMap["properties"].(map[string]any); ok {
						properties = props
					}
				}

				if len(properties) > 0 {
					builder.WriteString("    参数:\n")
					for paramName, paramInfo := range properties {
						paramDesc := ""
						paramType := ""

						if paramDetails, ok := paramInfo.(map[string]any); ok {
							if desc, ok := paramDetails["description"].(string); ok {
								paramDesc = desc
							}
							if t, ok := paramDetails["type"].(string); ok {
								paramType = t
							}
							builder.WriteString(fmt.Sprintf("      - %s (%s): %s\n", paramName, paramType, paramDesc))
						}
					}
				}
				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}

	if !hasTools {
		return ""
	}

	builder.WriteString("要使用工具，请使用以下格式:\n")
	builder.WriteString(fmt.Sprintf("<%s>\n", taskTag))
	builder.WriteString("{\n")
	builder.WriteString("  \"server\": \"服务器ID\",\n")
	builder.WriteString("  \"tool\": \"工具名称\",\n")
	builder.WriteString("  \"args\": {\"参数名\": 参数值, ...}\n")
	builder.WriteString("}\n")
	builder.WriteString(fmt.Sprintf("</%s>\n", taskTag))

	return builder.String()
}

// processToolCalls处理函数调用模式下的工具调用
func (c *MCPClient) processToolCalls(ctx context.Context, gen *Generation) error {
	if len(gen.ToolCalls) == 0 {
		return nil
	}

	if gen.GenerationInfo == nil {
		gen.GenerationInfo = make(map[string]any)
	}

	for _, call := range gen.ToolCalls {
		parts := strings.Split(call.Function.Name, ".")
		if len(parts) != 2 {
			continue
		}

		serverID := parts[0]
		toolName := parts[1]

		var args map[string]any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			continue
		}

		result, err := c.host.ExecuteTool(ctx, serverID, toolName, args)
		if err != nil {
			gen.GenerationInfo["tool_error_"+call.ID] = err.Error()
			continue
		}

		gen.GenerationInfo["tool_result_"+call.ID] = result.Content
	}

	return nil
}

// createMCPTools创建MCP工具定义
func (c *MCPClient) createMCPTools(ctx context.Context) []Tool {
	var tools []Tool
	connections := c.host.GetAllConnections()

	for serverID := range connections {
		toolsResult, err := c.host.ListTools(ctx, serverID)
		if err != nil {
			continue
		}

		for _, tool := range toolsResult.Tools {
			funcDef := &FunctionDefinition{
				Name:        fmt.Sprintf("%s.%s", serverID, tool.Name),
				Description: fmt.Sprintf("[服务器: %s] %s", serverID, tool.Description),
				Parameters:  tool.InputSchema,
			}

			tools = append(tools, Tool{
				Type:     "function",
				Function: funcDef,
			})
		}
	}

	return tools
}

// extractMCPTasks 从文本中提取MCP任务
func extractMCPTasks(content string, taskTag string) ([]MCPTask, error) {
	var tasks []MCPTask

	re := regexp.MustCompile(fmt.Sprintf(`<%s>\n(.*?)\n</%s>`, taskTag, taskTag))
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		taskJSON := match[1]

		// 解析JSON
		var task MCPTask
		if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
			continue
		}

		// 验证任务
		if task.Server == "" || task.Tool == "" {
			continue
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// taskToString 将任务转换为字符串
func taskToString(task MCPTask) string {
	bytes, _ := json.Marshal(task)
	return string(bytes)
}
