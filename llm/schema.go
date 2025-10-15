package llm

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

type LLM interface {
	// Generate生成文本回复
	Generate(ctx context.Context, messages []Message, options ...GenerateOption) (*Generation, error)
	// GenerateContent使用消息列表生成回复
	GenerateContent(ctx context.Context, messages []Message, options ...GenerateOption) (*Generation, error)
}

type (
	Message struct {
		Role             MessageRole `json:"role"`
		Name             string      `json:"name,omitempty"`
		Content          string      `json:"content"`
		ReasoningContent string      `json:"reasoning_content,omitempty"`
		ToolCallId       string      `json:"tool_call_id,omitempty"`
		ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
	}
	GuardResponse struct {
		OK          bool   `json:"ok"`
		Problems    string `json:"problems"`
		Suggestions string `json:"suggestions"`
	}
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type Generation struct {
	Role             string                                     `json:"role"`
	Content          string                                     `json:"content"`
	StopReason       string                                     `json:"stop_reason"`
	ReasoningContent string                                     `json:"reasoning_content,omitempty"`
	Messages         []openai.ChatCompletionMessage             `json:"messages,omitempty"`
	GenerationInfo   map[string]any                             `json:"generation_info,omitempty"`
	ToolCalls        []ToolCall                                 `json:"tool_calls,omitempty"`
	Usage            *Usage                                     `json:"usage,omitempty"`
	LogProbs         *openai.ChatCompletionStreamChoiceLogprobs `json:"logprobs,omitempty"`

	// MCP相关信息
	MCPWorkMode     LLMWorkMode `json:"-"` // 工作模式
	MCPTaskTag      string      `json:"-"` // 任务标签
	MCPResultTag    string      `json:"-"` // 结果标签
	MCPSystemPrompt string      `json:"-"` // 提示
}

// Usage 令牌使用情况
type Usage struct {
	CompletionTokens int `json:"completion_tokens,omitempty"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// ToolCall 模型调用工具的请求
type ToolCall struct {
	ID       string        `json:"id,omitempty"`
	Type     string        `json:"type,omitempty"`
	Function *FunctionCall `json:"function,omitempty"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// LLMWorkMode LLM的工作模式
type LLMWorkMode string

const (
	TextMode         LLMWorkMode = "text"          // 纯文本模式
	FunctionCallMode LLMWorkMode = "function_call" // 函数调用模式
)

func NewSystemMessage(name, content string) *Message {
	return &Message{
		Role:    RoleSystem,
		Name:    name,
		Content: content,
	}
}

func NewUserMessage(name, content string) *Message {
	return &Message{
		Role:    RoleUser,
		Name:    name,
		Content: content,
	}
}

func NewAssistantMessage(name, content string, toolCalls []ToolCall) *Message {
	return &Message{
		Role:      RoleAssistant,
		Name:      name,
		Content:   content,
		ToolCalls: toolCalls,
	}
}

func NewToolMessage(toolCallID, content string) *Message {
	return &Message{
		Role:       RoleTool,
		ToolCallId: toolCallID,
		Content:    content,
	}
}
