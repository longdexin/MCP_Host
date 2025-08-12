package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExecutionState 存储执行状态
type ExecutionState struct {
	gen             *Generation
	prompt          string
	opts            *GenerateOptions
	originalOptions []GenerateOption
	allTaskResults  []TaskResult
	executionRound  int
	capturedOutput  *strings.Builder
	currentGen      *Generation
}

// NewExecutionState 创建新的执行状态
func NewExecutionState(gen *Generation, prompt string, opts *GenerateOptions, originalOptions ...GenerateOption) *ExecutionState {
	capturedOutput := &strings.Builder{}
	capturedOutput.WriteString(gen.Content)

	maxRounds := opts.MCPMaxToolExecutionRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}

	return &ExecutionState{
		gen:             gen,
		prompt:          prompt,
		opts:            opts,
		originalOptions: originalOptions,
		allTaskResults:  []TaskResult{},
		executionRound:  0,
		capturedOutput:  capturedOutput,
		currentGen:      gen,
	}
}

// hasToolCalls 检查生成内容是否包含工具调用
func (c *MCPClient) hasToolCalls(gen *Generation) bool {
	return (gen.MCPWorkMode == TextMode && containsMCPTasks(gen.Content, gen.MCPTaskTag)) ||
		(gen.MCPWorkMode == FunctionCallMode && len(gen.ToolCalls) > 0)
}

// prepareOptions 准备选项
func (c *MCPClient) prepareOptions(options []GenerateOption) (*GenerateOptions, []GenerateOption) {
	opts := DefaultGenerateOption()
	for _, opt := range options {
		opt(opts)
	}
	return opts, options
}

// notifyExecutionStart 通知执行开始
func (c *MCPClient) notifyExecutionStart(ctx context.Context, state *ExecutionState) {
	if state.opts.StateNotifyFunc != nil {
		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "process_start",
			Stage: "start",
			Data:  map[string]any{"mode": state.gen.MCPWorkMode},
		})
	}
}

// executeToolsLoop 执行多轮工具调用循环
func (c *MCPClient) executeToolsLoop(ctx context.Context, state *ExecutionState) error {
	maxRounds := state.opts.MCPMaxToolExecutionRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}

	// 多轮工具执行循环
	for state.executionRound < maxRounds {
		state.executionRound++
		c.notifyRoundStart(ctx, state)
		hasExecutedTools, err := c.executeRound(ctx, state)
		if err != nil {
			return err
		}

		if !hasExecutedTools {
			break
		}

		if state.executionRound >= maxRounds {
			if err := c.getFinalResult(ctx, state); err != nil {
				return err
			}
			break
		}
		// 准备下一轮执行
		if err := c.prepareNextRound(ctx, state); err != nil {
			return err
		}
	}

	return nil
}

// notifyRoundStart 通知开始新一轮
func (c *MCPClient) notifyRoundStart(ctx context.Context, state *ExecutionState) {
	if state.opts.StateNotifyFunc != nil {
		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "execution_round",
			Stage: "start",
			Data: map[string]any{
				"round":      state.executionRound,
				"max_rounds": state.opts.MCPMaxToolExecutionRounds,
			},
		})
	}
}

// executeRound 执行单轮工具调用
func (c *MCPClient) executeRound(ctx context.Context, state *ExecutionState) (bool, error) {
	if state.currentGen.MCPWorkMode == TextMode {
		return c.executeTextModeRound(ctx, state)
	} else {
		return c.executeFunctionCallRound(ctx, state)
	}
}

// executeTextModeRound 执行文本模式下的工具调用
func (c *MCPClient) executeTextModeRound(ctx context.Context, state *ExecutionState) (bool, error) {
	// 提取任务
	c.notifyExtractingTasks(ctx, state, "start")

	tasks, roundTaskResults, err := c.processMCPTasksWithResults(ctx, state, state.currentGen.MCPTaskTag)
	if err != nil {
		return false, err
	}

	c.notifyExtractingTasks(ctx, state, "complete", len(roundTaskResults))

	if len(tasks) == 0 && len(roundTaskResults) == 0 {
		return false, nil
	}
	state.allTaskResults = append(state.allTaskResults, roundTaskResults...)

	// 输出结果
	if state.opts.StreamingFunc != nil {
		c.streamTextModeResults(ctx, state, roundTaskResults)
	}

	return true, nil
}

// executeFunctionCallRound 执行函数调用模式下的工具调用
func (c *MCPClient) executeFunctionCallRound(ctx context.Context, state *ExecutionState) (bool, error) {
	// 通知开始处理工具调用
	c.notifyProcessingToolCalls(ctx, state, "start")
	if err := c.processToolCalls(ctx, state.currentGen); err != nil {
		return false, err
	}

	c.notifyProcessingToolCalls(ctx, state, "complete")
	if len(state.currentGen.ToolCalls) == 0 {
		return false, nil
	}

	// 输出结果
	if state.opts.StreamingFunc != nil {
		c.streamFunctionCallResults(ctx, state)
	}

	return true, nil
}

// notifyExtractingTasks 通知任务提取状态
func (c *MCPClient) notifyExtractingTasks(ctx context.Context, state *ExecutionState, stage string, taskCount ...int) {
	if state.opts.StateNotifyFunc != nil {
		data := map[string]any{"round": state.executionRound}
		if len(taskCount) > 0 && stage == "complete" {
			data["task_count"] = taskCount[0]
		}

		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "extracting_tasks",
			Stage: stage,
			Data:  data,
		})
	}
}

// notifyProcessingToolCalls 通知工具调用处理状态
func (c *MCPClient) notifyProcessingToolCalls(ctx context.Context, state *ExecutionState, stage string) {
	if state.opts.StateNotifyFunc != nil {
		data := map[string]any{"round": state.executionRound}
		if stage == "start" {
			data["call_count"] = len(state.currentGen.ToolCalls)
		}

		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "processing_tool_calls",
			Stage: stage,
			Data:  data,
		})
	}
}

// streamTextModeResults 流式输出文本模式结果
func (c *MCPClient) streamTextModeResults(ctx context.Context, state *ExecutionState, results []TaskResult) {
	resultInfos := make([]MCPToolExecutionResult, 0, len(results))
	for _, result := range results {
		c.notifyToolCall(ctx, state, result.Task.Server, result.Task.Tool, "start", result.Task.Args)
		resultInfo := c.createToolExecutionResult(result)
		resultInfos = append(resultInfos, resultInfo)
		c.notifyToolResult(ctx, state, result)
	}
	if len(resultInfos) > 0 {
		fmt.Fprintf(state.capturedOutput, "<%s>", state.currentGen.MCPResultTag)
		_ = state.opts.StreamingFunc(ctx, nil, resultInfos)
	}
}

// createToolExecutionResult 创建工具执行结果
func (c *MCPClient) createToolExecutionResult(result TaskResult) MCPToolExecutionResult {
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

	return resultInfo
}

// streamFunctionCallResults 流式输出函数调用结果
func (c *MCPClient) streamFunctionCallResults(ctx context.Context, state *ExecutionState) {
	resultInfos := make([]MCPToolExecutionResult, 0, len(state.currentGen.ToolCalls))
	for _, call := range state.currentGen.ToolCalls {
		serverID, toolName, args := c.parseToolCall(call)
		c.notifyToolCall(ctx, state, serverID, toolName, "start", map[string]any{"call_id": call.ID})
		resultInfo := MCPToolExecutionResult{
			Server: serverID,
			Tool:   toolName,
			Args:   args,
			ID:     call.ID,
		}
		resultInfos = append(resultInfos, resultInfo)
		c.fillToolCallResult(state.currentGen, &resultInfo)
		c.notifyFunctionCallResult(ctx, state, resultInfo)
	}

	if len(resultInfos) > 0 {
		fmt.Fprintf(state.capturedOutput, "<%s>", state.currentGen.MCPResultTag)
		_ = state.opts.StreamingFunc(ctx, nil, resultInfos)
	}
}

// parseToolCall 解析工具调用
func (c *MCPClient) parseToolCall(call ToolCall) (string, string, map[string]any) {
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
	return serverID, toolName, args
}

// fillToolCallResult 填充工具调用结果
func (c *MCPClient) fillToolCallResult(gen *Generation, resultInfo *MCPToolExecutionResult) {
	if errStr, ok := gen.GenerationInfo["tool_error_"+resultInfo.ID].(string); ok && errStr != "" {
		resultInfo.Status = "error"
		resultInfo.Error = errStr
	} else if result, ok := gen.GenerationInfo["tool_result_"+resultInfo.ID]; ok {
		resultInfo.Status = "success"
		resultInfo.Result = result
	}
}

// notifyToolCall 通知工具调用状态
func (c *MCPClient) notifyToolCall(ctx context.Context, state *ExecutionState, serverID, toolName, stage string, data map[string]any) {
	if state.opts.StateNotifyFunc != nil {
		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:     "tool_call",
			ServerID: serverID,
			ToolName: toolName,
			Stage:    stage,
			Data:     data,
		})
	}
}

// notifyToolResult 通知工具结果状态
func (c *MCPClient) notifyToolResult(ctx context.Context, state *ExecutionState, result TaskResult) {
	if state.opts.StateNotifyFunc != nil {
		stateData := map[string]any{}
		if result.Error != "" {
			stateData["error"] = result.Error
		} else {
			stateData["result"] = result.Result
		}

		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:     "tool_result",
			ServerID: result.Task.Server,
			ToolName: result.Task.Tool,
			Stage:    "complete",
			Data:     stateData,
		})
	}
}

// notifyFunctionCallResult 通知函数调用结果状态
func (c *MCPClient) notifyFunctionCallResult(ctx context.Context, state *ExecutionState, resultInfo MCPToolExecutionResult) {
	if state.opts.StateNotifyFunc != nil {
		stateData := map[string]any{"call_id": resultInfo.ID}
		if resultInfo.Error != "" {
			stateData["error"] = resultInfo.Error
		} else {
			stateData["result"] = resultInfo.Result
		}

		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:     "tool_result",
			ServerID: resultInfo.Server,
			ToolName: resultInfo.Tool,
			Stage:    "complete",
			Data:     stateData,
		})
	}
}

// prepareNextRound 准备下一轮执行
func (c *MCPClient) prepareNextRound(ctx context.Context, state *ExecutionState) error {
	intermediateMessages := c.buildIntermediateMessages(ctx, state)

	c.notifyIntermediateGeneration(ctx, state, "start")
	intermediateOpts := c.createIntermediateOptions(state)

	nextGen, err := c.llm.GenerateContent(ctx, intermediateMessages, intermediateOpts...)
	if err != nil {
		c.notifyIntermediateGenerationError(ctx, state, err)
		return err
	}

	c.notifyIntermediateGeneration(ctx, state, "complete")
	nextGen.MCPWorkMode = state.gen.MCPWorkMode
	nextGen.MCPTaskTag = state.gen.MCPTaskTag
	nextGen.MCPResultTag = state.gen.MCPResultTag
	nextGen.MCPPrompt = state.gen.MCPPrompt
	state.currentGen = nextGen

	return nil
}

func (c *MCPClient) getFinalResult(ctx context.Context, state *ExecutionState) error {
	intermediateMessages := c.buildFinalResultMessages(ctx, state)

	c.notifyIntermediateGeneration(ctx, state, "start")
	intermediateOpts := c.createIntermediateOptions(state)

	nextGen, err := c.llm.GenerateContent(ctx, intermediateMessages, intermediateOpts...)
	if err != nil {
		c.notifyIntermediateGenerationError(ctx, state, err)
		return err
	}

	c.notifyIntermediateGeneration(ctx, state, "complete")
	nextGen.MCPWorkMode = state.gen.MCPWorkMode
	nextGen.MCPTaskTag = state.gen.MCPTaskTag
	nextGen.MCPResultTag = state.gen.MCPResultTag
	nextGen.MCPPrompt = state.gen.MCPPrompt
	state.currentGen = nextGen

	state.capturedOutput.WriteString(nextGen.Content)

	return nil
}

// buildIntermediateMessages 构建中间消息
func (c *MCPClient) buildIntermediateMessages(ctx context.Context, state *ExecutionState) []Message {
	if state.currentGen.MCPWorkMode == TextMode {
		return c.buildTextModeIntermediateMessages(ctx, state)
	} else {
		return c.buildFunctionCallIntermediateMessages(state)
	}
}

// buildIntermediateMessages 构建中间消息
func (c *MCPClient) buildFinalResultMessages(ctx context.Context, state *ExecutionState) []Message {
	if state.currentGen.MCPWorkMode == TextMode {
		return c.buildTextModeFinalResultMessages(ctx, state)
	} else {
		return c.buildFunctionCallFinalResultMessages(state)
	}
}

// buildTextModeIntermediateMessages 构建文本模式中间消息
func (c *MCPClient) buildTextModeIntermediateMessages(ctx context.Context, state *ExecutionState) []Message {
	var messages []Message

	systemMsg := NewSystemMessage("", state.currentGen.MCPPrompt)
	toolsInfo := c.formatMCPToolsAsText(ctx, state.currentGen.MCPTaskTag, state.opts.MCPDisabledTools...)
	if toolsInfo != "" {
		systemMsg.Content += "\n\n" + toolsInfo
	}
	messages = append(messages, *systemMsg)
	messages = append(messages, *NewUserMessage("", state.prompt))

	// 添加工具结果
	for _, result := range state.allTaskResults {
		var toolMsg string
		if result.Error != "" {
			toolMsg = fmt.Sprintf(c.toolErrorMsgTemplate, result.Task.Server, result.Task.Tool, result.Error)
		} else {
			resultJSON, _ := json.Marshal(result.Result)
			toolMsg = fmt.Sprintf(c.toolResultMsgTemplate, result.Task.Server, result.Task.Tool, string(resultJSON))
		}

		messages = append(messages, *NewUserMessage("", toolMsg))
	}

	// 添加额外指导
	remainingRounds := state.opts.MCPMaxToolExecutionRounds - state.executionRound
	if remainingRounds > 0 {
		guidanceMsg := fmt.Sprintf(c.nextRoundMsgTemplate, remainingRounds)
		messages = append(messages, *NewUserMessage("", guidanceMsg))
	}

	return messages
}

// buildTextModeIntermediateMessages 构建文本模式中间消息
func (c *MCPClient) buildTextModeFinalResultMessages(ctx context.Context, state *ExecutionState) []Message {
	var messages []Message

	systemMsg := NewSystemMessage("", state.currentGen.MCPPrompt)
	toolsInfo := c.formatMCPToolsAsText(ctx, state.currentGen.MCPTaskTag, state.opts.MCPDisabledTools...)
	if toolsInfo != "" {
		systemMsg.Content += "\n\n" + toolsInfo
	}
	messages = append(messages, *systemMsg)
	messages = append(messages, *NewUserMessage("", state.prompt))

	// 添加工具结果
	for _, result := range state.allTaskResults {
		var toolMsg string
		if result.Error != "" {
			toolMsg = fmt.Sprintf(c.toolErrorMsgTemplate, result.Task.Server, result.Task.Tool, result.Error)
		} else {
			resultJSON, _ := json.Marshal(result.Result)
			toolMsg = fmt.Sprintf(c.toolResultMsgTemplate, result.Task.Server, result.Task.Tool, string(resultJSON))
		}

		messages = append(messages, *NewUserMessage("", toolMsg))
	}

	// 添加额外指导
	guidanceMsg := c.finalResultMsgTemplate
	messages = append(messages, *NewUserMessage("", guidanceMsg))

	return messages
}

// buildFunctionCallIntermediateMessages 构建函数调用模式中间消息
func (c *MCPClient) buildFunctionCallIntermediateMessages(state *ExecutionState) []Message {
	var messages []Message
	systemMsg := NewSystemMessage("", c.functionCallSystemPrompt)
	messages = append(messages, *systemMsg)
	messages = append(messages, *NewUserMessage("", c.userQuestionTemplate+state.prompt))
	assistantMsg := NewAssistantMessage("", "", state.currentGen.ToolCalls)
	messages = append(messages, *assistantMsg)

	// 添加工具结果
	for _, call := range state.currentGen.ToolCalls {
		var resultContent string
		if errStr, ok := state.currentGen.GenerationInfo["tool_error_"+call.ID].(string); ok && errStr != "" {
			resultContent = fmt.Sprintf("Error: %s", errStr)
		} else if result, ok := state.currentGen.GenerationInfo["tool_result_"+call.ID]; ok {
			resultJSON, _ := json.Marshal(result)
			resultContent = string(resultJSON)
		} else {
			continue
		}

		messages = append(messages, *NewToolMessage(call.ID, resultContent))
	}

	// 添加额外指导
	remainingRounds := state.opts.MCPMaxToolExecutionRounds - state.executionRound
	if remainingRounds > 0 {
		guidanceMsg := fmt.Sprintf("You can call additional tools if needed (up to %d more rounds). Please continue your analysis.", remainingRounds)
		messages = append(messages, *NewUserMessage("", guidanceMsg))
	}

	return messages
}

// buildFunctionCallFinalMessages 构建函数调用模式最终消息
func (c *MCPClient) buildFunctionCallFinalResultMessages(state *ExecutionState) []Message {
	var messages []Message
	systemMsg := NewSystemMessage("", c.functionCallSystemPrompt)
	messages = append(messages, *systemMsg)
	messages = append(messages, *NewUserMessage("", c.userQuestionTemplate+state.prompt))
	assistantMsg := NewAssistantMessage("", "", state.currentGen.ToolCalls)
	messages = append(messages, *assistantMsg)

	// 添加工具结果
	for _, call := range state.currentGen.ToolCalls {
		var resultContent string
		if errStr, ok := state.currentGen.GenerationInfo["tool_error_"+call.ID].(string); ok && errStr != "" {
			resultContent = fmt.Sprintf("Error: %s", errStr)
		} else if result, ok := state.currentGen.GenerationInfo["tool_result_"+call.ID]; ok {
			resultJSON, _ := json.Marshal(result)
			resultContent = string(resultJSON)
		} else {
			continue
		}

		messages = append(messages, *NewToolMessage(call.ID, resultContent))
	}

	// 添加额外指导
	guidanceMsg := c.finalResultMsgTemplate
	messages = append(messages, *NewUserMessage("", guidanceMsg))

	return messages
}

// notifyIntermediateGeneration 通知中间生成状态
func (c *MCPClient) notifyIntermediateGeneration(ctx context.Context, state *ExecutionState, stage string) {
	if state.opts.StateNotifyFunc != nil {
		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "intermediate_generation",
			Stage: stage,
			Data:  map[string]any{"round": state.executionRound},
		})
	}
}

// notifyIntermediateGenerationError 通知中间生成错误
func (c *MCPClient) notifyIntermediateGenerationError(ctx context.Context, state *ExecutionState, err error) {
	if state.opts.StateNotifyFunc != nil {
		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "intermediate_generation",
			Stage: "error",
			Data:  map[string]any{"error": err.Error(), "round": state.executionRound},
		})
	}
}

// createIntermediateOptions 创建中间选项
func (c *MCPClient) createIntermediateOptions(state *ExecutionState) []GenerateOption {
	intermediateOpts := make([]GenerateOption, 0)
	for _, opt := range state.originalOptions {
		if !isAutoExecuteOption(opt) {
			intermediateOpts = append(intermediateOpts, opt)
		}
	}

	if state.opts.StreamingFunc != nil {
		intermediateStreamFunc := func(ctx context.Context, chunk []byte, toolResults []MCPToolExecutionResult) error {
			state.capturedOutput.Write(chunk)
			return state.opts.StreamingFunc(ctx, chunk, nil)
		}
		intermediateOpts = append(intermediateOpts, WithStreamingFunc(intermediateStreamFunc))
	}

	return intermediateOpts
}

// mergeGenerationInfo 合并生成信息
func (c *MCPClient) mergeGenerationInfo(finalGen *Generation, state *ExecutionState) {
	if finalGen.GenerationInfo == nil {
		finalGen.GenerationInfo = make(map[string]any)
	}

	if len(state.allTaskResults) > 0 {
		finalGen.GenerationInfo["mcp_task_results"] = state.allTaskResults
		finalGen.GenerationInfo["mcp_execution_rounds"] = state.executionRound
	} else {
		for k, v := range state.gen.GenerationInfo {
			if strings.HasPrefix(k, "tool_result_") || strings.HasPrefix(k, "tool_error_") {
				finalGen.GenerationInfo[k] = v
			}
		}
	}
}

// notifyProcessComplete 通知处理完成
func (c *MCPClient) notifyProcessComplete(ctx context.Context, state *ExecutionState) {
	if state.opts.StateNotifyFunc != nil {
		_ = state.opts.StateNotifyFunc(ctx, MCPExecutionState{
			Type:  "process_complete",
			Stage: "complete",
			Data: map[string]any{
				"has_results":      len(state.allTaskResults) > 0 || len(state.gen.ToolCalls) > 0,
				"mode":             state.gen.MCPWorkMode,
				"execution_rounds": state.executionRound,
			},
		})
	}
}
