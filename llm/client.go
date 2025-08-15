package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient OpenAI LLM的实现
type OpenAIClient struct {
	client         *openai.Client
	model          string
	responseFormat *openai.ChatCompletionResponseFormat
}

// OpenAIOption OpenAI客户端的配置选项
type OpenAIOption func(*openAIOptions)

type openAIOptions struct {
	token        string
	model        string
	baseURL      string
	organization string
	apiType      openai.APIType
	httpClient   *http.Client
	apiVersion   string
}

var _ LLM = (*OpenAIClient)(nil)

// NewOpenAIClient 创建一个新的OpenAI LLM客户端
func NewOpenAIClient(opts ...OpenAIOption) (*OpenAIClient, error) {
	options := &openAIOptions{
		apiType:    openai.APITypeOpenAI,
		httpClient: http.DefaultClient,
		model:      "gpt-4o",
	}

	if token := os.Getenv("OPENAI_API_KEY"); token != "" {
		options.token = token
	}
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		options.model = model
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		options.baseURL = baseURL
	}
	if org := os.Getenv("OPENAI_ORGANIZATION"); org != "" {
		options.organization = org
	}

	for _, opt := range opts {
		opt(options)
	}

	if options.token == "" {
		return nil, errors.New("missing OpenAI API key")
	}

	// 创建客户端配置
	config := openai.DefaultConfig(options.token)
	if options.apiType == openai.APITypeAzure {
		config = openai.DefaultAzureConfig(options.token, options.baseURL)
	}

	// 设置配置选项
	if options.baseURL != "" {
		config.BaseURL = options.baseURL
	}
	if options.organization != "" {
		config.OrgID = options.organization
	}
	if options.httpClient != nil {
		config.HTTPClient = options.httpClient
	}
	if options.apiVersion != "" {
		config.APIVersion = options.apiVersion
	}

	// 创建客户端
	client := openai.NewClientWithConfig(config)

	return &OpenAIClient{
		client: client,
		model:  options.model,
	}, nil
}

// Generate 生成文本回复
func (c *OpenAIClient) Generate(ctx context.Context, prompt string, options ...GenerateOption) (*Generation, error) {
	message := NewUserMessage("", prompt)
	return c.GenerateContent(ctx, []Message{*message}, options...)
}

// GenerateContent 使用消息列表生成回复
func (c *OpenAIClient) GenerateContent(ctx context.Context, messages []Message, options ...GenerateOption) (*Generation, error) {
	opts := DefaultGenerateOption()
	for _, opt := range options {
		opt(opts)
	}

	// 转换消息格式
	msgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		var toolCalls []openai.ToolCall
		if len(msg.ToolCalls) > 0 {
			toolCalls = make([]openai.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolCalls = append(toolCalls, openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolType(tc.Type),
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}

		if msg.Content == "" {
			msg.Content = " "
		}

		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:       string(msg.Role),
			Name:       msg.Name,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallId,
			ToolCalls:  toolCalls,
		})
	}

	// 创建请求
	req := openai.ChatCompletionRequest{
		Model:            c.model,
		Messages:         msgs,
		Temperature:      opts.Temperature,
		MaxTokens:        opts.MaxTokens,
		N:                opts.N,
		FrequencyPenalty: opts.FrequencyPenalty,
		PresencePenalty:  opts.PresencePenalty,
		Stop:             opts.StopWords,
		Stream:           opts.StreamingFunc != nil,
		TopP:             opts.TopP,
		// StreamOptions: &openai.StreamOptions{
		// 	IncludeUsage: true,
		// },
		ToolChoice:         opts.ToolChoice,
		LogProbs:           opts.LogProbs,
		TopLogProbs:        opts.TopLogProbs,
		Seed:               &opts.Seed,
		Metadata:           opts.Metadata,
		ChatTemplateKwargs: opts.ChatTemplateKwargs,
	}

	if opts.StreamingFunc != nil {
		req.StreamOptions = &openai.StreamOptions{
			IncludeUsage: true,
		}
	}

	if opts.JSONMode {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: "json_object"}
	}

	// 添加工具
	for _, tool := range opts.Tools {
		openaiTool := openai.Tool{
			Type: openai.ToolType(tool.Type),
		}

		if tool.Function != nil {
			openaiTool.Function = &openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
				Strict:      tool.Function.Strict,
			}
		}

		req.Tools = append(req.Tools, openaiTool)
	}

	// 设置并行工具调用
	if opts.ParallelToolCalls != nil && len(req.Tools) > 0 {
		req.ParallelToolCalls = opts.ParallelToolCalls
	}

	if c.responseFormat != nil {
		req.ResponseFormat = c.responseFormat
	}

	// 处理流式响应
	if opts.StreamingFunc != nil {
		return c.handleStreamResponse(ctx, req, opts)
	}

	// 处理非流式响应
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("no completion choices returned")
	}

	choice := resp.Choices[0]
	gen := &Generation{
		Role:             choice.Message.Role,
		Content:          choice.Message.Content,
		StopReason:       string(choice.FinishReason),
		ReasoningContent: choice.Message.ReasoningContent,
		Messages: []openai.ChatCompletionMessage{
			choice.Message,
		},
	}

	// 处理工具调用
	if len(choice.Message.ToolCalls) > 0 {
		gen.ToolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))
		for _, tc := range choice.Message.ToolCalls {
			gen.ToolCalls = append(gen.ToolCalls, ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: &FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	if resp.Usage.TotalTokens > 0 {
		gen.Usage = &Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return gen, nil
}

// handleStreamResponse 处理流式响应
func (c *OpenAIClient) handleStreamResponse(ctx context.Context, req openai.ChatCompletionRequest, opts *GenerateOptions) (*Generation, error) {
	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion stream: %w", err)
	}
	defer stream.Close()

	gen := &Generation{
		Usage:          &Usage{},
		GenerationInfo: make(map[string]any),
	}

	contentSb, reasoningContentSb := new(strings.Builder), new(strings.Builder)
	noRole := true
	for {
		resp, err := stream.Recv()
		if errors.Is(err, context.Canceled) {
			return gen, nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return gen, fmt.Errorf("context deadline exceeded: %w", err)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("error receiving from stream: %w", err)
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]

		// 更新生成内容
		if noRole {
			gen.Role = choice.Delta.Role
			noRole = false
		}

		contentSb.WriteString(choice.Delta.Content)
		reasoningContentSb.WriteString(choice.Delta.ReasoningContent)

		if choice.FinishReason != "" {
			gen.StopReason = string(choice.FinishReason)
		}

		// 处理工具调用
		if len(choice.Delta.ToolCalls) > 0 {
			c.processToolCallsStream(gen, choice.Delta.ToolCalls)
		}

		// 调用流式回调
		if opts.StreamingFunc != nil {
			if err := opts.StreamingFunc(ctx, &choice.Delta, nil); err != nil {
				return gen, fmt.Errorf("streaming function returned error: %w", err)
			}
		}

		// 更新使用情况
		if resp.Usage != nil {
			gen.Usage.PromptTokens = resp.Usage.PromptTokens
			gen.Usage.CompletionTokens = resp.Usage.CompletionTokens
			gen.Usage.TotalTokens = resp.Usage.TotalTokens
		}
	}
	gen.Content = contentSb.String()
	gen.ReasoningContent = reasoningContentSb.String()
	gen.Messages = append(gen.Messages, openai.ChatCompletionMessage{
		Role:             gen.Role,
		Content:          gen.Content,
		ReasoningContent: gen.ReasoningContent,
	})
	return gen, nil
}

// processToolCallsStream 处理流式工具调用
func (c *OpenAIClient) processToolCallsStream(gen *Generation, toolCalls []openai.ToolCall) {
	if len(gen.ToolCalls) == 0 {
		gen.ToolCalls = make([]ToolCall, 0, len(toolCalls))
	}

	for _, call := range toolCalls {
		var found bool
		var toolCall *ToolCall
		var idx int

		for i, tc := range gen.ToolCalls {
			if tc.ID == call.ID {
				toolCall = &gen.ToolCalls[i]
				found = true
				idx = i
				break
			}
		}

		if !found {
			idx = len(gen.ToolCalls)
			gen.ToolCalls = append(gen.ToolCalls, ToolCall{
				ID:       call.ID,
				Type:     string(call.Type),
				Function: &FunctionCall{},
			})
			toolCall = &gen.ToolCalls[idx]
		}

		// 更新工具调用
		if call.Function.Name != "" {
			toolCall.Function.Name += call.Function.Name
		}
		if call.Function.Arguments != "" {
			toolCall.Function.Arguments += call.Function.Arguments
		}
	}
}

// WithToken 设置OpenAI API令牌
func WithToken(token string) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.token = token
	}
}

// WithOpenAIModel 设置OpenAI模型
func WithOpenAIModel(model string) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.model = model
	}
}

// WithBaseURL 设置OpenAI基础URL
func WithBaseURL(baseURL string) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.baseURL = baseURL
	}
}

// WithOrganization 设置OpenAI组织ID
func WithOrganization(org string) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.organization = org
	}
}

// WithAPIType 设置API类型
func WithAPIType(apiType openai.APIType) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.apiType = apiType
	}
}

// WithAPIVersion设置API版本
func WithAPIVersion(version string) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.apiVersion = version
	}
}

// WithHTTPClient 设置HTTP客户端
func WithHTTPClient(client *http.Client) OpenAIOption {
	return func(opts *openAIOptions) {
		opts.httpClient = client
	}
}
