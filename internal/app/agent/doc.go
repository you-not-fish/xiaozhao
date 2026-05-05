// Package agent 是 Agent Orchestrator 的实现。
//
// 它把"加载会话上下文 -> 调模型 -> 解析工具调用 -> 执行工具 -> 把结果回传模型 -> 直至模型给出终态"
// 的核心循环封装成一个流式 Run 接口。事件先持久化到 response_events 表，
// 再通过 channel 推给 SSE handler，保证前端断线后仍可重放。
//
// 设计要点：
//   1. 所有 IO 都向接口编程：模型 → model.Provider；工具 → tool.Registry；
//      会话/消息/事件/审计 → domain.Repository。这样 P1 阶段把 mock 换 OpenAI、
//      把 builtin 工具换成 MCP / sandbox 时都不需要改 Orchestrator。
//   2. 写事件 → 推给 SSE，顺序不能反：断线重连后客户端只能从持久化里恢复。
//   3. 工具循环最大轮数受 Config.Agent.MaxToolIterations 控制（默认 5）。
//   4. 工具结果体积超过阈值会截断，并在事件 metadata 里标 truncated=true，
//      避免单次结果撑爆模型上下文。
//   5. RunRequest 预留 AgentID / RunMode 等字段，给后续多 Agent 协作留口子。
package agent
