package llm

// 默认的MCP提示模板
const defaultMCPPrompt = `You are now an MCP AI assistant.
When I give you a task, if you need to call external tools or services, please put your tool call request inside <MCP_HOST_TASK> and </MCP_HOST_TASK> tags.
Please strictly use the following format:
<MCP_HOST_TASK>
{"server":"serverId", "tool":"toolName", "args":{parameters}}
</MCP_HOST_TASK>

For example, if you need to get the current time from server "server1", you should return:
<MCP_HOST_TASK>
{"server":"server1", "tool":"get_current_time", "args":{}}
</MCP_HOST_TASK>

You should think first and provide your answer, then suggest using tools. Don't immediately call tools at the beginning of your response.

Make sure your response is clear, accurate, and strictly follows the format above.`

// 工具执行错误消息模板
const defaultToolErrorMessageTemplate = `Tool execution error: %s.%s
Error: %s`

// 工具执行结果消息模板
const defaultToolResultMessageTemplate = `I have used tool %s.%s to get the following information:
%s`

// 函数调用模式下的系统提示
const defaultFunctionCallSystemPrompt = `You are an AI assistant that can use tools to help users solve problems. Please provide a complete response based on the tool call results.`
