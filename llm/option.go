package llm

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// GenerateOption是配置GenerateOptions的函数
type GenerateOption func(*GenerateOptions)

// GenerateOptions是调用模型的选项集。不同模型可能支持不同的选项
type GenerateOptions struct {
	Model              string                                                                                                               `json:"model"`                         // 模型名称
	CandidateCount     int                                                                                                                  `json:"candidate_count"`               // 生成的候选回复数量
	MaxTokens          int                                                                                                                  `json:"max_tokens"`                    // 生成的最大令牌数
	Temperature        float32                                                                                                              `json:"temperature"`                   // 采样温度，介于0和1之间
	StopWords          []string                                                                                                             `json:"stop_words"`                    // 停止词列表
	StreamingFunc      func(ctx context.Context, delta *openai.ChatCompletionStreamChoiceDelta, toolResults []MCPToolExecutionResult) error `json:"-"`                             // 流式响应的回调函数
	TopK               int                                                                                                                  `json:"top_k"`                         // Top-K采样的令牌数量
	TopP               float32                                                                                                              `json:"top_p"`                         // Top-P采样的累积概率
	Seed               int                                                                                                                  `json:"seed"`                          // 确定性采样的种子
	MinLength          int                                                                                                                  `json:"min_length"`                    // 生成文本的最小长度
	MaxLength          int                                                                                                                  `json:"max_length"`                    // 生成文本的最大长度
	N                  int                                                                                                                  `json:"n"`                             // 为每个输入消息生成多少个完成选项
	RepetitionPenalty  float32                                                                                                              `json:"repetition_penalty"`            // 重复惩罚
	FrequencyPenalty   float32                                                                                                              `json:"frequency_penalty"`             // 频率惩罚
	PresencePenalty    float32                                                                                                              `json:"presence_penalty"`              // 存在惩罚
	JSONMode           bool                                                                                                                 `json:"json"`                          // JSON模式
	Tools              []Tool                                                                                                               `json:"tools,omitempty"`               // 可用工具列表
	ParallelToolCalls  *bool                                                                                                                `json:"parallel_tool_calls,omitempty"` // 是否启用并行工具调用
	ToolChoice         any                                                                                                                  `json:"tool_choice"`                   // 工具选择
	Metadata           map[string]string                                                                                                    `json:"metadata,omitempty"`            // 请求的元数据
	ChatTemplateKwargs map[string]any                                                                                                       `json:"chat_template_kwargs"`          // 模板参数
	ResponseMIMEType   string                                                                                                               `json:"response_mime_type,omitempty"`  // 响应MIME类型
	LogProbs           bool                                                                                                                 `json:"logprobs,omitempty"`            // 是否记录概率
	TopLogProbs        int                                                                                                                  `json:"top_logprobs,omitempty"`        // 返回每个位置最可能的令牌数量

	// MCP相关选项
	MCPWorkMode               LLMWorkMode `json:"-"` // LLM工作模式
	MCPAutoExecute            bool        `json:"-"` // 是否自动执行MCP工具调用
	MCPTaskTag                string      `json:"-"` // MCP任务标签，默认为 MCP_HOST_TASK
	MCPResultTag              string      `json:"-"` // MCP结果标签，默认为 MCP_HOST_RESULT
	MCPDisabledTools          []string    `json:"-"` // 禁用的工具列表，格式为 "serverID.toolName"
	MCPMaxToolExecutionRounds int         `json:"-"` // 最大工具执行轮次

	StateNotifyFunc        StateNotifyFunc `json:"-"` // 状态通知回调
	EnableDebug            bool            // 启动调试，主要用来打印即将发送的消息
	DisableTips            bool            // 禁用每轮工具调用后添加提示词
	SystemPromptTemplate   string          // 默认提示
	ToolErrorMsgTemplate   string          // 工具错误消息模板
	ToolResultMsgTemplate  string          // 工具结果消息模板
	NextRoundMsgTemplate   string          // 下一轮分析消息模板
	FinalResultMsgTemplate string          // 最终答案消息模板
}

// Tool 模型可以使用的工具
type Tool struct {
	Type     string              `json:"type"`               // 工具类型
	Function *FunctionDefinition `json:"function,omitempty"` // 函数定义
}

// FunctionDefinition是模型可以调用的函数的定义
type FunctionDefinition struct {
	Name        string `json:"name"`                 // 函数名称
	Description string `json:"description"`          // 函数描述
	Parameters  any    `json:"parameters,omitempty"` // 函数参数
	Strict      bool   `json:"strict,omitempty"`     // 是否严格调用。仅用于OpenAI LLM结构化输出
}

// ToolChoice是选择使用的特定工具
type ToolChoice struct {
	Type     string             `json:"type"`               // 工具类型
	Function *FunctionReference `json:"function,omitempty"` // 函数引用（如果工具是函数）
}

// FunctionReference是对函数的引用
type FunctionReference struct {
	Name string `json:"name"` // 函数名称
}

// FunctionCallBehavior是调用函数时的行为
type FunctionCallBehavior string

const (
	// FunctionCallBehaviorNone不会调用任何函数
	FunctionCallBehaviorNone FunctionCallBehavior = "none"
	// FunctionCallBehaviorAuto会自动调用函数
	FunctionCallBehaviorAuto FunctionCallBehavior = "auto"
)

const (
	MCP_DEFAULT_TASK_TAG   = "MCP_HOST_TASK"   // 默认任务标签
	MCP_DEFAULT_RESULT_TAG = "MCP_HOST_RESULT" // 默认结果标签
)

// WithModel 指定要使用的模型名称
func WithModel(model string) GenerateOption {
	return func(o *GenerateOptions) {
		o.Model = model
	}
}

// WithMaxTokens 指定生成的最大令牌数
func WithMaxTokens(maxTokens int) GenerateOption {
	return func(o *GenerateOptions) {
		o.MaxTokens = maxTokens
	}
}

// WithCandidateCount 指定生成的候选回复数量
func WithCandidateCount(c int) GenerateOption {
	return func(o *GenerateOptions) {
		o.CandidateCount = c
	}
}

// WithTemperature 指定模型温度
func WithTemperature(temperature float32) GenerateOption {
	return func(o *GenerateOptions) {
		o.Temperature = temperature
	}
}

// WithStopWords 指定停止生成的单词列表
func WithStopWords(stopWords []string) GenerateOption {
	return func(o *GenerateOptions) {
		o.StopWords = stopWords
	}
}

// WithOptions 指定选项
func WithOptions(options GenerateOptions) GenerateOption {
	return func(o *GenerateOptions) {
		*o = options
	}
}

// WithStreamingFunc 指定流式响应的回调函数
func WithStreamingFunc(streamingFunc func(ctx context.Context, delta *openai.ChatCompletionStreamChoiceDelta, toolResults []MCPToolExecutionResult) error) GenerateOption {
	return func(o *GenerateOptions) {
		o.StreamingFunc = streamingFunc
	}
}

// WithMCPWorkMode 指定MCP的工作模式
func WithMCPWorkMode(mode LLMWorkMode) GenerateOption {
	return func(o *GenerateOptions) {
		o.MCPWorkMode = mode
	}
}

// WithTools 指定要使用的工具
func WithTools(tools []Tool) GenerateOption {
	return func(o *GenerateOptions) {
		o.Tools = tools
	}
}

// WithMCPAutoExecute 指定是否自动执行MCP工具调用
func WithMCPAutoExecute(autoExecute bool) GenerateOption {
	return func(o *GenerateOptions) {
		o.MCPAutoExecute = autoExecute
	}
}

// WithMCPTaskTag 指定MCP任务的标签
func WithMCPTaskTag(tag string) GenerateOption {
	return func(o *GenerateOptions) {
		o.MCPTaskTag = tag
	}
}

// WithMCPResultTag 指定MCP结果的标签
func WithMCPResultTag(tag string) GenerateOption {
	return func(o *GenerateOptions) {
		o.MCPResultTag = tag
	}
}

// WithParallelToolCalls 通知回调
func WithStateNotifyFunc(notifyFunc StateNotifyFunc) GenerateOption {
	return func(o *GenerateOptions) {
		o.StateNotifyFunc = notifyFunc
	}
}

// WithMCPDisabledTools 指定要禁用的MCP工具列表
func WithMCPDisabledTools(disabledTools []string) GenerateOption {
	return func(o *GenerateOptions) {
		o.MCPDisabledTools = disabledTools
	}
}

// WithMCPMaxToolExecutionRounds 指定MCP最大工具执行轮次
func WithMCPMaxToolExecutionRounds(rounds int) GenerateOption {
	return func(o *GenerateOptions) {
		if rounds > 0 {
			o.MCPMaxToolExecutionRounds = rounds
		}
	}
}

// DefaultGenerateOption返回默认的生成选项
func DefaultGenerateOption() *GenerateOptions {
	return &GenerateOptions{
		ParallelToolCalls:         nil,
		MCPWorkMode:               TextMode,
		MCPAutoExecute:            false, // 默认不自动执行
		MCPTaskTag:                MCP_DEFAULT_TASK_TAG,
		MCPResultTag:              MCP_DEFAULT_RESULT_TAG,
		MCPMaxToolExecutionRounds: 5,
		SystemPromptTemplate:      defaultSystemPromptTemplate,
		ToolErrorMsgTemplate:      defaultToolErrorMessageTemplate,
		ToolResultMsgTemplate:     defaultToolResultMessageTemplate,
		NextRoundMsgTemplate:      defaultNextRoundMsgTemplate,
		FinalResultMsgTemplate:    defaultFinalResultMsgTemplate,
	}
}

// MCPExecutionState  MCP执行状态
type MCPExecutionState struct {
	Type     string         // "tool_call", "tool_result", "llm_response", "execution_round", "intermediate_generation" 等
	Stage    string         // "start", "complete", "error" 等
	ServerID string         // 对于tool_call和tool_result, 服务器ID
	ToolName string         // 对于tool_call和tool_result, 工具名称
	Data     map[string]any // 其他相关数据
}

type StateNotifyFunc func(ctx context.Context, state MCPExecutionState) error
