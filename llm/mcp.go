package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/longdexin/MCP_Host"
)

// MCPTask MCP任务
type MCPTask struct {
	Server string         `json:"server"`        // ServerID
	Tool   string         `json:"tool"`          // 工具名称
	Args   map[string]any `json:"args"`          // 参数
	Text   string         `json:"text,optional"` // 原始任务字符串
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
func (c *MCPClient) processMCPTasksWithResults(ctx context.Context, state *ExecutionState, taskTag ...string) ([]TaskResult, error) {
	tag := MCP_DEFAULT_TASK_TAG
	if len(taskTag) > 0 && taskTag[0] != "" {
		tag = taskTag[0]
	}
	if state.gen.MCPTaskTag != "" {
		tag = state.gen.MCPTaskTag
	}
	executedTaskTextMap := make(map[string]struct{}, len(state.allTaskResults))
	for _, taskResult := range state.allTaskResults {
		executedTaskTextMap[taskResult.Task.Text] = struct{}{}
	}
	tasks, err := c.ExtractMCPTasks(state.gen.Content, tag)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, nil
	}

	var results []TaskResult

	// 执行任务
TASK_LOOP:
	for _, task := range tasks {
		// 如果已经执行过了，就跳过
		if _, ok := executedTaskTextMap[task.Text]; ok {
			continue TASK_LOOP
		}
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
	builder.WriteString("Available tools:\n\n")

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
			builder.WriteString(fmt.Sprintf("Server '%s':\n", serverID))

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
					builder.WriteString("    Parameters:\n")
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

	builder.WriteString("Please follow the format below to call the tool:\n")
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

// 标签正则表达式匹配
func taskRegex(taskTag string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(?s)<%s>\s*(.*?)\s*</%s>`, taskTag, taskTag))
}

// containsMCPTasks 检查内容中是否包含MCP任务
func containsMCPTasks(content string, taskTag string) bool {
	re := taskRegex(taskTag)
	return re.MatchString(content)
}

// extractMCPTasks 从文本中提取MCP任务
func extractMCPTasks(content string, taskTag string) ([]MCPTask, error) {
	var tasks []MCPTask

	re := taskRegex(taskTag)

	// 只需要最后一个</think>标签后的文本
	if lastIndex := strings.LastIndex(content, "</think>"); lastIndex >= 0 {
		content = content[lastIndex+8:]
	}

	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		taskJSON := match[1]

		taskJSON = strings.TrimSpace(taskJSON)

		// 解析JSON
		var task MCPTask
		if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
			continue
		}

		// 验证任务
		if task.Server == "" || task.Tool == "" {
			continue
		}

		task.Text = taskJSON

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
	if !c.hasToolCalls(gen) {
		return gen, nil
	}

	opts, originalOptions := c.prepareOptions(options)
	state := NewExecutionState(gen, prompt, opts, originalOptions...)
	c.notifyExecutionStart(ctx, state)

	// 执行工具调用循环
	if err := c.executeToolsLoop(ctx, state); err != nil {
		return nil, err
	}

	finalGen := &Generation{
		Content:        state.capturedOutput.String(),
		MCPWorkMode:    state.gen.MCPWorkMode,
		MCPTaskTag:     state.gen.MCPTaskTag,
		MCPResultTag:   state.gen.MCPResultTag,
		MCPPrompt:      state.gen.MCPPrompt,
		GenerationInfo: make(map[string]any),
	}

	c.mergeGenerationInfo(finalGen, state)
	c.notifyProcessComplete(ctx, state)

	return finalGen, nil
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
