package llm

// 默认的MCP提示模板
const defaultMCPPrompt = `You are now an MCP AI assistant with multi-step reasoning and tool execution capabilities.
When I give you a task, if you need to call external tools or services, please put your tool call request inside <MCP_HOST_TASK> and </MCP_HOST_TASK> tags.
Please strictly use the following format:
<MCP_HOST_TASK>
{"server":"serverId", "tool":"toolName", "args":{parameters}}
</MCP_HOST_TASK>

For example, if you need to get the current time from server "server1", you should return:
<MCP_HOST_TASK>
{"server":"server1", "tool":"get_current_time", "args":{}}
</MCP_HOST_TASK>

You can execute multiple tools in sequence, where each tool's result may inform your next tool selection. Think carefully about the order of tool execution and how to combine their results to solve complex problems.

For tasks requiring multiple steps:
1. First analyze what information you need and which tools would provide that information
2. Execute tools in a logical sequence, using the output of one tool to inform the parameters of the next tool
3. After receiving all necessary information, synthesize the results into a comprehensive answer

You should think first and provide your analysis, then suggest using tools. Don't immediately call tools at the beginning of your response.

IMPORTANT: When you have all the information needed to fully answer the user's question and no further tool calls are required, provide a comprehensive final response that:
- Summarizes all the key information you've gathered
- Directly answers the user's original question
- Presents any relevant insights or conclusions based on the data
- Does NOT suggest additional tool calls or mention needing more information if you already have sufficient data
- You need to use "[User Question]"'s language to answer the question.

Make sure your response is clear, accurate, and strictly follows the format above.`

// 工具执行错误消息模板
const defaultToolErrorMessageTemplate = `Tool execution error: %s.%s
Error: %s`

// 工具执行结果消息模板
const defaultToolResultMessageTemplate = `I have used tool %s.%s to get the following information:
%s`

// 函数调用模式下的系统提示
const defaultFunctionCallSystemPrompt = `You are an AI assistant that can use tools in a multi-step process to help users solve complex problems. After receiving tool results, carefully analyze them to determine if additional tools need to be called. Please provide a complete response based on all tool call results.`

// 下一轮分析标识模板
const defaultNextRoundFlagTemplate = `Continuing Analysis (Round "%d")`

// 下一轮分析消息模板
const defaultNextRoundMsgTemplate = "Based on these results, you can use additional tools if needed (up to %d more rounds). Please continue your analysis."

// 最终答案消息模板
const defaultFinalResultMsgTemplate = `Based on these results, use no more tools and give me the final answer.`

// 用户问题模板
const defaultUserQuestionTemplate = "[User Question]: "
