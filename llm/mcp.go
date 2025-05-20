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
	Server string         `json:"server"` // ServerID
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
	llm                      LLM               // 底层LLM客户端
	host                     *MCP_Host.MCPHost // MCP主机
	prompt                   string            // 默认提示
	toolErrorMsgTemplate     string            // 工具错误消息模板
	toolResultMsgTemplate    string            // 工具结果消息模板
	functionCallSystemPrompt string            // 函数调用模式的系统提示
}

// NewMCPClient 创建一个新的MCPClient
func NewMCPClient(llm LLM, host *MCP_Host.MCPHost) *MCPClient {
	return &MCPClient{
		llm:                      llm,
		host:                     host,
		prompt:                   defaultMCPPrompt,
		toolErrorMsgTemplate:     defaultToolErrorMessageTemplate,
		toolResultMsgTemplate:    defaultToolResultMessageTemplate,
		functionCallSystemPrompt: defaultFunctionCallSystemPrompt,
	}
}

// SetPrompt 设置默认提示
func (c *MCPClient) SetPrompt(prompt string) {
	c.prompt = prompt
}

// SetToolErrorMessageTemplate 设置工具错误消息模板
func (c *MCPClient) SetToolErrorMessageTemplate(template string) {
	c.toolErrorMsgTemplate = template
}

// SetToolResultMessageTemplate 设置工具结果消息模板
func (c *MCPClient) SetToolResultMessageTemplate(template string) {
	c.toolResultMsgTemplate = template
}

// SetFunctionCallSystemPrompt 设置函数调用模式的系统提示
func (c *MCPClient) SetFunctionCallSystemPrompt(prompt string) {
	c.functionCallSystemPrompt = prompt
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
		toolsInfo := c.formatMCPToolsAsText(ctx, opts.MCPTaskTag, opts.MCPDisabledTools...)

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

		// 存储MCP相关信息，以便在后续处理中使用
		gen.MCPWorkMode = opts.MCPWorkMode
		gen.MCPTaskTag = opts.MCPTaskTag
		gen.MCPResultTag = opts.MCPResultTag
		gen.MCPPrompt = mcpPrompt

		// 如果启用了自动执行并存在工具调用，则处理工具调用并生成最终回复
		if opts.MCPAutoExecute {
			return c.ExecuteAndFeedback(ctx, gen, prompt, options...)
		}

		return gen, nil
	} else {
		// 函数调用模式
		tools := c.createMCPTools(ctx, opts.MCPDisabledTools...)

		toolsOption := WithTools(tools)
		gen, err := c.llm.Generate(ctx, prompt, append(options, toolsOption)...)
		if err != nil {
			return nil, err
		}

		// 存储MCP相关信息
		gen.MCPWorkMode = opts.MCPWorkMode
		gen.MCPTaskTag = opts.MCPTaskTag

		if opts.MCPAutoExecute && len(gen.ToolCalls) > 0 {
			return c.ExecuteAndFeedback(ctx, gen, prompt, options...)
		} else if !opts.MCPAutoExecute {
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

	var userPrompt string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			userPrompt = messages[i].Content
			break
		}
	}

	// 在文本模式下添加MCP提示
	if opts.MCPWorkMode == TextMode {
		mcpPrompt := opts.MCPPrompt
		if mcpPrompt == "" {
			mcpPrompt = c.prompt
		}

		toolsInfo := c.formatMCPToolsAsText(ctx, opts.MCPTaskTag, opts.MCPDisabledTools...)

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

		// 存储MCP相关信息
		gen.MCPWorkMode = opts.MCPWorkMode
		gen.MCPTaskTag = opts.MCPTaskTag
		gen.MCPResultTag = opts.MCPResultTag
		gen.MCPPrompt = mcpPrompt

		if opts.MCPAutoExecute {
			return c.ExecuteAndFeedback(ctx, gen, userPrompt, options...)
		}

		return gen, nil
	} else {
		// 函数调用模式
		tools := c.createMCPTools(ctx, opts.MCPDisabledTools...)
		toolsOption := WithTools(tools)

		gen, err := c.llm.GenerateContent(ctx, messages, append(options, toolsOption)...)
		if err != nil {
			return nil, err
		}

		// 存储MCP相关信息
		gen.MCPWorkMode = opts.MCPWorkMode
		gen.MCPTaskTag = opts.MCPTaskTag

		if opts.MCPAutoExecute && len(gen.ToolCalls) > 0 {
			return c.ExecuteAndFeedback(ctx, gen, userPrompt, options...)
		} else if !opts.MCPAutoExecute {
			c.appendToolCallsToContent(gen, opts.MCPTaskTag)
		}

		return gen, nil
	}
}

// processMCPTasksWithResults 解析并执行任务，返回结果列表
func (c *MCPClient) processMCPTasksWithResults(ctx context.Context, gen *Generation, taskTag ...string) ([]TaskResult, error) {
	tag := MCP_DEFAULT_TASK_TAG
	if len(taskTag) > 0 && taskTag[0] != "" {
		tag = taskTag[0]
	}
	if gen.MCPTaskTag != "" {
		tag = gen.MCPTaskTag
	}

	tasks, err := c.ExtractMCPTasks(gen.Content, tag)
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
func (c *MCPClient) ExecuteMCPTasksWithResults(ctx context.Context, content string, taskTag ...string) ([]TaskResult, error) {
	tag := MCP_DEFAULT_TASK_TAG
	if len(taskTag) > 0 && taskTag[0] != "" {
		tag = taskTag[0]
	}

	// 提取MCP任务
	tasks, err := c.ExtractMCPTasks(content, tag)
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
func (c *MCPClient) ExecuteMCPTasks(ctx context.Context, content string, taskTag ...string) (string, error) {
	tag := MCP_DEFAULT_TASK_TAG
	if len(taskTag) > 0 && taskTag[0] != "" {
		tag = taskTag[0]
	}

	// 提取MCP任务
	tasks, err := c.ExtractMCPTasks(content, tag)
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
				fmt.Sprintf("<%s>\n%s\n</%s>", tag, taskToString(task), tag),
				fmt.Sprintf("<%s>\n%s\n[ERROR] %v\n</%s>", tag, taskToString(task), err, tag),
				1,
			)
			continue
		}

		resultStr, _ := json.Marshal(result.Content)
		updatedContent = strings.Replace(
			updatedContent,
			fmt.Sprintf("<%s>\n%s\n</%s>", tag, taskToString(task), tag),
			fmt.Sprintf("<%s>\n%s\n[RESULT] %s\n</%s>", tag, taskToString(task), string(resultStr), tag),
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
func (c *MCPClient) ExtractMCPTasks(content string, taskTag ...string) ([]MCPTask, error) {
	tag := MCP_DEFAULT_TASK_TAG
	if len(taskTag) > 0 && taskTag[0] != "" {
		tag = taskTag[0]
	}
	return extractMCPTasks(content, tag)
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
func (c *MCPClient) formatMCPToolsAsText(ctx context.Context, taskTag string, disabledTools ...string) string {
	var builder strings.Builder
	builder.WriteString("可用工具列表:\n\n")

	connections := c.host.GetAllConnections()

	hasTools := false

	// 创建禁用工具的快速查找表
	disabledToolsMap := make(map[string]bool)
	for _, dt := range disabledTools {
		disabledToolsMap[dt] = true
	}

	for serverID := range connections {
		toolsResult, err := c.host.ListTools(ctx, serverID)
		if err != nil {
			continue
		}

		serverHasTools := false
		for _, tool := range toolsResult.Tools {
			toolFullName := fmt.Sprintf("%s.%s", serverID, tool.Name)
			if !disabledToolsMap[toolFullName] {
				serverHasTools = true
				break
			}
		}

		if !serverHasTools {
			continue
		}

		if len(toolsResult.Tools) > 0 {
			hasTools = true
			builder.WriteString(fmt.Sprintf("服务器 %s:\n", serverID))

			for _, tool := range toolsResult.Tools {
				toolFullName := fmt.Sprintf("%s.%s", serverID, tool.Name)
				if disabledToolsMap[toolFullName] {
					continue
				}

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

	builder.WriteString("Want to use a tool? Please use the following format:\n")
	builder.WriteString(fmt.Sprintf("<%s>\n", taskTag))
	builder.WriteString("{\n")
	builder.WriteString("  \"server\": \"Server ID\",\n")
	builder.WriteString("  \"tool\": \"Tool Name\",\n")
	builder.WriteString("  \"args\": {\"arg1\": \"value1\",\n")
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
func (c *MCPClient) createMCPTools(ctx context.Context, disabledTools ...string) []Tool {
	var tools []Tool
	connections := c.host.GetAllConnections()

	disabledToolsMap := make(map[string]bool)
	for _, dt := range disabledTools {
		disabledToolsMap[dt] = true
	}

	for serverID := range connections {
		toolsResult, err := c.host.ListTools(ctx, serverID)
		if err != nil {
			continue
		}

		for _, tool := range toolsResult.Tools {
			toolFullName := fmt.Sprintf("%s.%s", serverID, tool.Name)
			if disabledToolsMap[toolFullName] {
				continue
			}

			funcDef := &FunctionDefinition{
				Name:        fmt.Sprintf("%s.%s", serverID, tool.Name),
				Description: fmt.Sprintf("[Server: %s] %s", serverID, tool.Description),
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

// MCPToolExecutionResult MCP工具执行结果的JSON表示
type MCPToolExecutionResult struct {
	Server string         `json:"server"`           // 服务器ID
	Tool   string         `json:"tool"`             // 工具名称
	Args   map[string]any `json:"args"`             // 调用参数
	Status string         `json:"status"`           // 状态："success" 或 "error"
	Result any            `json:"result,omitempty"` // 执行结果，如果成功
	Error  string         `json:"error,omitempty"`  // 错误信息，如果失败
	ID     string         `json:"id,omitempty"`     // 工具调用ID（函数调用模式）
}

// ExecuteAndFeedback 执行工具调用并将结果反馈给LLM生成最终回复
func (c *MCPClient) ExecuteAndFeedback(ctx context.Context, gen *Generation, prompt string, options ...GenerateOption) (*Generation, error) {
	if (gen.MCPWorkMode == TextMode && !containsMCPTasks(gen.Content, gen.MCPTaskTag)) ||
		(gen.MCPWorkMode == FunctionCallMode && len(gen.ToolCalls) == 0) {
		return gen, nil
	}

	opts := DefaultGenerateOption()
	for _, opt := range options {
		opt(opts)
	}

	stateNotify := opts.StateNotifyFunc

	if stateNotify != nil {
		_ = stateNotify(ctx, MCPExecutionState{
			Type:  "process_start",
			Stage: "start",
			Data:  map[string]any{"mode": gen.MCPWorkMode},
		})
	}

	streamingFunc := opts.StreamingFunc
	var taskResults []TaskResult

	var capturedOutput strings.Builder
	capturedOutput.WriteString(gen.Content)

	if gen.MCPWorkMode == TextMode {
		var err error
		if stateNotify != nil {
			_ = stateNotify(ctx, MCPExecutionState{
				Type:  "extracting_tasks",
				Stage: "start",
			})
		}

		taskResults, err = c.processMCPTasksWithResults(ctx, gen, gen.MCPTaskTag)
		if err != nil {
			return nil, err
		}

		if stateNotify != nil {
			_ = stateNotify(ctx, MCPExecutionState{
				Type:  "extracting_tasks",
				Stage: "complete",
				Data:  map[string]any{"task_count": len(taskResults)},
			})
		}

		if len(taskResults) == 0 {
			return gen, nil
		}

		if streamingFunc != nil && len(taskResults) > 0 {
			capturedOutput.WriteString("\n")
			_ = streamingFunc(ctx, []byte("\n"))

			for _, result := range taskResults {
				if stateNotify != nil {
					_ = stateNotify(ctx, MCPExecutionState{
						Type:     "tool_call",
						ServerID: result.Task.Server,
						ToolName: result.Task.Tool,
						Stage:    "start",
						Data:     result.Task.Args,
					})
				}

				var resultOutput strings.Builder
				resultOutput.WriteString(fmt.Sprintf("<%s>\n", opts.MCPResultTag))

				resultInfo := MCPToolExecutionResult{
					Server: result.Task.Server,
					Tool:   result.Task.Tool,
					Args:   result.Task.Args,
				}

				if result.Error != "" {
					resultInfo.Status = "error"
					resultInfo.Error = result.Error
				} else {
					resultInfo.Status = "success"
					resultInfo.Result = result.Result
				}

				resultJSON, _ := json.Marshal(resultInfo)
				resultOutput.WriteString(string(resultJSON))
				resultOutput.WriteString("\n")
				resultOutput.WriteString(fmt.Sprintf("</%s>\n", opts.MCPResultTag))

				if stateNotify != nil {
					state := MCPExecutionState{
						Type:     "tool_result",
						ServerID: result.Task.Server,
						ToolName: result.Task.Tool,
						Stage:    "complete",
					}
					if result.Error != "" {
						state.Data = map[string]any{"error": result.Error}
					} else {
						state.Data = result.Result
					}
					_ = stateNotify(ctx, state)
				}
				resultStr := resultOutput.String()
				capturedOutput.WriteString(resultStr)
				_ = streamingFunc(ctx, []byte(resultStr))
			}

			separatorStr := "\n\n"
			capturedOutput.WriteString(separatorStr)
			_ = streamingFunc(ctx, []byte(separatorStr))
		}
	} else {
		if stateNotify != nil {
			_ = stateNotify(ctx, MCPExecutionState{
				Type:  "processing_tool_calls",
				Stage: "start",
				Data:  map[string]any{"call_count": len(gen.ToolCalls)},
			})
		}

		if err := c.processToolCalls(ctx, gen); err != nil {
			return nil, err
		}

		if stateNotify != nil {
			_ = stateNotify(ctx, MCPExecutionState{
				Type:  "processing_tool_calls",
				Stage: "complete",
			})
		}

		if streamingFunc != nil && len(gen.ToolCalls) > 0 {
			capturedOutput.WriteString("\n")
			_ = streamingFunc(ctx, []byte("\n"))

			resultOutput := strings.Builder{}
			resultOutput.WriteString(fmt.Sprintf("<%s>\n", opts.MCPResultTag))

			for _, call := range gen.ToolCalls {
				serverID := ""
				toolName := ""
				var args map[string]any

				parts := strings.Split(call.Function.Name, ".")
				if len(parts) == 2 {
					serverID = parts[0]
					toolName = parts[1]
				} else {
					serverID = "unknown"
					toolName = call.Function.Name
				}

				_ = json.Unmarshal([]byte(call.Function.Arguments), &args)

				if stateNotify != nil {
					_ = stateNotify(ctx, MCPExecutionState{
						Type:     "tool_call",
						ServerID: serverID,
						ToolName: toolName,
						Stage:    "start",
						Data:     map[string]any{"call_id": call.ID},
					})
				}

				resultInfo := MCPToolExecutionResult{
					Server: serverID,
					Tool:   toolName,
					Args:   args,
					ID:     call.ID,
				}

				if errStr, ok := gen.GenerationInfo["tool_error_"+call.ID].(string); ok && errStr != "" {
					resultInfo.Status = "error"
					resultInfo.Error = errStr

					if stateNotify != nil {
						_ = stateNotify(ctx, MCPExecutionState{
							Type:     "tool_result",
							ServerID: serverID,
							ToolName: toolName,
							Stage:    "complete",
							Data:     map[string]any{"error": errStr, "call_id": call.ID},
						})
					}
				} else if result, ok := gen.GenerationInfo["tool_result_"+call.ID]; ok {
					resultInfo.Status = "success"
					resultInfo.Result = result

					if stateNotify != nil {
						_ = stateNotify(ctx, MCPExecutionState{
							Type:     "tool_result",
							ServerID: serverID,
							ToolName: toolName,
							Stage:    "complete",
							Data:     map[string]any{"result": result, "call_id": call.ID},
						})
					}
				}

				resultJSON, _ := json.Marshal(resultInfo)
				resultOutput.WriteString(string(resultJSON))
				resultOutput.WriteString("\n")
			}

			resultOutput.WriteString(fmt.Sprintf("</%s>\n", opts.MCPResultTag))
			resultStr := resultOutput.String()
			capturedOutput.WriteString(resultStr)
			_ = streamingFunc(ctx, []byte(resultStr))

			separatorStr := "\n\n"
			capturedOutput.WriteString(separatorStr)
			_ = streamingFunc(ctx, []byte(separatorStr))
		}
	}

	var capturingStreamingFunc func(ctx context.Context, chunk []byte) error
	if streamingFunc != nil {
		capturingStreamingFunc = func(ctx context.Context, chunk []byte) error {
			capturedOutput.Write(chunk)
			return streamingFunc(ctx, chunk)
		}
	}

	// 通知开始构建消息上下文
	if stateNotify != nil {
		_ = stateNotify(ctx, MCPExecutionState{
			Type:  "building_context",
			Stage: "start",
		})
	}

	var messages []Message

	if gen.MCPWorkMode == TextMode {
		systemMsg := NewSystemMessage("", gen.MCPPrompt)
		messages = append(messages, *systemMsg)
		messages = append(messages, *NewUserMessage("", prompt))

		for _, result := range taskResults {
			var toolMsg string
			if result.Error != "" {
				toolMsg = fmt.Sprintf(c.toolErrorMsgTemplate,
					result.Task.Server, result.Task.Tool, result.Error)
			} else {
				resultJSON, _ := json.Marshal(result.Result)
				toolMsg = fmt.Sprintf(c.toolResultMsgTemplate,
					result.Task.Server, result.Task.Tool, string(resultJSON))
			}

			messages = append(messages, *NewUserMessage("", toolMsg))
		}
	} else {
		systemMsg := NewSystemMessage("", c.functionCallSystemPrompt)
		messages = append(messages, *systemMsg)
		messages = append(messages, *NewUserMessage("", prompt))
		assistantMsg := NewAssistantMessage("", "", gen.ToolCalls)
		messages = append(messages, *assistantMsg)

		for _, call := range gen.ToolCalls {
			var resultContent string
			if errStr, ok := gen.GenerationInfo["tool_error_"+call.ID].(string); ok && errStr != "" {
				resultContent = fmt.Sprintf("Error: %s", errStr)
			} else if result, ok := gen.GenerationInfo["tool_result_"+call.ID]; ok {
				resultJSON, _ := json.Marshal(result)
				resultContent = string(resultJSON)
			} else {
				continue
			}

			messages = append(messages, *NewToolMessage(call.ID, resultContent))
		}
	}

	// 通知消息上下文构建完成
	if stateNotify != nil {
		_ = stateNotify(ctx, MCPExecutionState{
			Type:  "building_context",
			Stage: "complete",
			Data:  map[string]any{"message_count": len(messages)},
		})
	}

	finalOptions := make([]GenerateOption, 0)
	for _, opt := range options {
		if !isAutoExecuteOption(opt) {
			finalOptions = append(finalOptions, opt)
		}
	}
	if capturingStreamingFunc != nil {
		finalOptions = append(finalOptions, WithStreamingFunc(capturingStreamingFunc))
	}

	// 通知开始生成最终回复
	if stateNotify != nil {
		_ = stateNotify(ctx, MCPExecutionState{
			Type:  "generating_response",
			Stage: "start",
		})
	}

	fmt.Printf("\n\n\n\n生成最终回复 \n\n%v\n\n--------\n\n", messages)
	finalGen, err := c.llm.GenerateContent(ctx, messages, finalOptions...)
	if err != nil {
		// 通知生成响应失败
		if stateNotify != nil {
			_ = stateNotify(ctx, MCPExecutionState{
				Type:  "generating_response",
				Stage: "error",
				Data:  map[string]any{"error": err.Error()},
			})
		}
		return nil, err
	}

	// 通知生成响应成功
	if stateNotify != nil {
		_ = stateNotify(ctx, MCPExecutionState{
			Type:  "generating_response",
			Stage: "complete",
			Data:  map[string]any{"content_length": len(finalGen.Content)},
		})
	}

	finalGen.Content = capturedOutput.String()

	// 保留原始调用的相关信息
	finalGen.MCPWorkMode = gen.MCPWorkMode
	finalGen.MCPTaskTag = gen.MCPTaskTag
	finalGen.MCPResultTag = gen.MCPResultTag
	finalGen.MCPPrompt = gen.MCPPrompt

	// 合并信息
	if finalGen.GenerationInfo == nil {
		finalGen.GenerationInfo = make(map[string]any)
	}
	if len(taskResults) > 0 {
		finalGen.GenerationInfo["mcp_task_results"] = taskResults
	} else {
		for k, v := range gen.GenerationInfo {
			if strings.HasPrefix(k, "tool_result_") || strings.HasPrefix(k, "tool_error_") {
				finalGen.GenerationInfo[k] = v
			}
		}
	}

	// 通知整个处理过程完成
	if stateNotify != nil {
		_ = stateNotify(ctx, MCPExecutionState{
			Type:  "process_complete",
			Stage: "complete",
			Data: map[string]any{
				"has_results": len(taskResults) > 0 || len(gen.ToolCalls) > 0,
				"mode":        gen.MCPWorkMode,
			},
		})
	}

	return finalGen, nil
}

// containsMCPTasks 检查内容中是否包含MCP任务
func containsMCPTasks(content string, taskTag string) bool {
	re := regexp.MustCompile(fmt.Sprintf(`<%s>\n.*?\n</%s>`, taskTag, taskTag))
	return re.MatchString(content)
}

// isAutoExecuteOption 检查是否是自动执行选项
func isAutoExecuteOption(opt GenerateOption) bool {
	testOpt := DefaultGenerateOption()
	opt(testOpt)
	return testOpt.MCPAutoExecute
}

// taskToString 将任务转换为字符串
func taskToString(task MCPTask) string {
	bytes, _ := json.Marshal(task)
	return string(bytes)
}
