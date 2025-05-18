package llm

import "context"

// GenerateOption是配置GenerateOptions的函数
type GenerateOption func(*GenerateOptions)

// GenerateOptions是调用模型的选项集。不同模型可能支持不同的选项
type GenerateOptions struct {
	Model                  string                                        `json:"model"`                         // 模型名称
	CandidateCount         int                                           `json:"candidate_count"`               // 生成的候选回复数量
	MaxTokens              int                                           `json:"max_tokens"`                    // 生成的最大令牌数
	Temperature            float32                                       `json:"temperature"`                   // 采样温度，介于0和1之间
	StopWords              []string                                      `json:"stop_words"`                    // 停止词列表
	StreamingFunc          func(ctx context.Context, chunk []byte) error `json:"-"`                             // 流式响应的回调函数
	ReasoningStreamingFunc func(ctx context.Context, chunk []byte) error `json:"-"`                             // 推理过程的流式响应回调函数
	TopK                   int                                           `json:"top_k"`                         // Top-K采样的令牌数量
	TopP                   float64                                       `json:"top_p"`                         // Top-P采样的累积概率
	Seed                   int                                           `json:"seed"`                          // 确定性采样的种子
	MinLength              int                                           `json:"min_length"`                    // 生成文本的最小长度
	MaxLength              int                                           `json:"max_length"`                    // 生成文本的最大长度
	N                      int                                           `json:"n"`                             // 为每个输入消息生成多少个完成选项
	RepetitionPenalty      float32                                       `json:"repetition_penalty"`            // 重复惩罚
	FrequencyPenalty       float32                                       `json:"frequency_penalty"`             // 频率惩罚
	PresencePenalty        float32                                       `json:"presence_penalty"`              // 存在惩罚
	JSONMode               bool                                          `json:"json"`                          // JSON模式
	Tools                  []Tool                                        `json:"tools,omitempty"`               // 可用工具列表
	ParallelToolCalls      *bool                                         `json:"parallel_tool_calls,omitempty"` // 是否启用并行工具调用
	ToolChoice             any                                           `json:"tool_choice"`                   // 工具选择
	Metadata               map[string]string                             `json:"metadata,omitempty"`            // 请求的元数据
	ResponseMIMEType       string                                        `json:"response_mime_type,omitempty"`  // 响应MIME类型
	LogProbs               bool                                          `json:"logprobs,omitempty"`            // 是否记录概率
	TopLogProbs            int                                           `json:"top_logprobs,omitempty"`        // 返回每个位置最可能的令牌数量

	// MCP相关选项
	MCPWorkMode    LLMWorkMode `json:"-"` // LLM工作模式
	MCPPrompt      string      `json:"-"` // 在文本模式下使用的提示
	MCPAutoExecute bool        `json:"-"` // 是否自动执行MCP工具调用
	MCPTaskTag     string      `json:"-"` // MCP任务标签，默认为 MCP_HOST_TASK
}

// Tool表示模型可以使用的工具
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
	// 默认的MCP任务标签
	MCP_DEFAULT_TASK_TAG = "MCP_HOST_TASK"
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
func WithStreamingFunc(streamingFunc func(ctx context.Context, chunk []byte) error) GenerateOption {
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

// WithMCPPrompt 指定MCP在文本模式下使用的提示
func WithMCPPrompt(prompt string) GenerateOption {
	return func(o *GenerateOptions) {
		o.MCPPrompt = prompt
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

// DefaultGenerateOption返回默认的生成选项
func DefaultGenerateOption() *GenerateOptions {
	return &GenerateOptions{
		ParallelToolCalls: nil,
		MCPWorkMode:       TextMode,
		MCPPrompt:         defaultMCPPrompt,
		MCPAutoExecute:    false,           // 默认不自动执行
		MCPTaskTag:        "MCP_HOST_TASK", // 默认任务标签
	}
}
