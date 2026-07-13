# Eino + AI SDK Vue 在 Koko Terminal Agent 中的可行性与接入方法

> 结论性质：技术调研与 PoC 建议，不替代 [Koko SSH Terminal Agent 设计稿](./ssh-agent-design.md) 中已经接受的安全、执行与审计约束。
>
> 调研日期：2026-07-13
>
> Eino 源码基线：`/opt/codes/eino@922b6a8a233b5233fe47eecee6cd2c005e8c39cd`（2026-07-08，位于 `v0.9.12` 之后 3 个提交）
>
> 建议 PoC 版本：`github.com/cloudwego/eino v0.9.12`，不直接依赖 `main` 或 `v0.10.0-alpha.*`

## 1. 结论

将 Eino 与 Koko 的 Go 后端、Vue 3 前端和 `ai` / `@ai-sdk/vue` 结合实现 Terminal Agent，**技术上可行，建议先做受限 PoC**。

推荐的定位不是“用 Eino 替换 Koko Agent 领域层”，而是：

- Eino 作为可替换的 **Agent 编排引擎适配器**，负责模型循环、流式模型输出、tool-call 调度、中断/恢复机制和回调扩展点；
- Koko 继续作为 **安全和业务事实源**，负责 `PlanState`、tool catalog、权限、风险、审批、幂等、active PTY binding、InputLease、命令审计和事件持久化；
- `@ai-sdk/vue` 作为 **浏览器 UI 状态投影**，通过自定义 `ChatTransport` 消费 Koko 的 canonical event；它不直接消费 Eino `AgentEvent`，也不成为服务端状态事实源。

建议选择“选择性集成”，不要直接采用 Eino `DeepAgent`、内置 shell/filesystem 工具或默认 `planexecute` 作为生产 Terminal Agent。

| 方案 | 结论 | 原因 |
| --- | --- | --- |
| Koko 自研全部 Agent loop | 可行，成本较高 | 边界最可控，但要自行实现流、tool loop、interrupt/resume、middleware 和大量契约测试 |
| **Koko 领域层 + Eino ADK 编排适配器** | **推荐 PoC** | 复用成熟 Go Agent 机制，同时不牺牲 Koko 的安全、审计和 active PTY 不变量 |
| Eino `planexecute` / `DeepAgent` 端到端接管 | 不推荐 | 计划、工具、执行、持久化语义与 Koko 的版本化 Plan、审批 digest 和单 PTY 串行约束不等价 |

## 2. 当前代码基础

### 2.1 Koko

Koko 已具备接入所需的主体技术栈：

- 后端为 Go，当前 `go.mod` 使用 Go `1.26.0`，高于 Eino 要求的 Go `1.18+`；
- 前端为 Vue `3.5.x`、TypeScript、Pinia、Naive UI 和 xterm；
- 已有 terminal WebSocket：`pkg/httpd/webserver.go`、`ui/src/hooks/useTerminalSocket.ts`；
- 已有旧 Chat AI WebSocket：`pkg/httpd/chat.go`、`pkg/httpd/webrouter.go`；
- active PTY 的真实写入链路位于 `pkg/proxy/switch.go`，输入经过 `Parser.ParseUserInput` 后才写入 `srvConn`；
- 命令 ACL/review、服务端输出解析和命令记录位于 `pkg/proxy/parser.go` 等现有链路。

旧 `pkg/httpd/chat.go` 只适合普通问答过渡，不能演进为生产 Agent Runtime。它目前使用进程内 `sync.Map`、截断后的 8 轮 QA、布尔 interrupt、两分钟 context timeout 和 `go-openai` Chat Completions 流，没有 Plan、tool、审批、持久化恢复、序列重放或 active terminal 可信绑定。

### 2.2 Eino

本次源码基线提供以下可复用能力：

| 能力 | 主要源码 | 对 Koko 的价值 |
| --- | --- | --- |
| `adk.ChatModelAgent` | `adk/chatmodel.go` | 内置 ReAct 循环、模型/tool 交替、最大迭代数、middleware |
| `adk.Runner` | `adk/runner.go` | 统一启动、流式 `AgentEvent`、checkpoint resume |
| Agent event | `adk/interface.go` | 模型流、tool result、interrupt、error 的运行时事件入口 |
| HITL | `adk/interrupt.go`、`compose/interrupt.go` | Stateful interrupt、checkpoint、按 interrupt address 恢复 |
| Tool node | `compose/tool_node.go` | tool schema、middleware、参数预处理、未知工具处理和顺序控制 |
| Callback | `callbacks/`、`utils/callbacks/` | 模型/tool/graph/agent 的 trace、metrics 和审计观测入口 |
| Graph/Workflow | `compose/` | 需要确定性工作流时可组合节点，不必都交给模型决定 |
| Plan/Task/Deep 预构建 | `adk/prebuilt/planexecute/`、`adk/middlewares/plantask/`、`adk/prebuilt/deep/` | 可作设计参考，但不应直接成为 Koko 的 Plan 权威实现 |

Eino 核心仓库只提供抽象和编排。模型 Provider 的实际实现位于独立的 `cloudwego/eino-ext`；因此“引入 Eino”不自动等于“已经支持 Koko 要求的 OpenAI Responses API、兼容端点差异和 `store:false`”。Provider adapter 仍须单独做能力核对和契约测试。

## 3. 可行性矩阵

| 维度 | 可行性 | 判断 |
| --- | --- | --- |
| Go 编译与部署 | 高 | Go 版本满足；无须增加 Node 后端/sidecar |
| ReAct/tool loop | 高 | `ChatModelAgent` 可直接承担通用循环 |
| 模型流式输出 | 高 | Eino message stream 可映射到 Koko event，再投影到 Vue |
| Vue / `@ai-sdk/vue` | 高 | Vue 版本满足 peer dependency；`ChatTransport` 明确允许 WebSocket 等自定义协议 |
| Tool schema 与调用 | 高 | 可用 Eino tool interface 包装 Koko application service |
| active PTY | 中 | Eino 不理解 Koko PTY，需要完整的 Koko tool adapter、binding 和 lease |
| 审批/HITL | 中高 | Eino interrupt/resume 可承载暂停机制，但授权判断、digest 和审计必须由 Koko 实现 |
| 权威 PlanState | 中低 | Eino 默认 plan 只有字符串步骤，不满足 Koko step ID/revision/status/evidence/digest 契约 |
| 多实例恢复 | 中 | Eino 只定义 checkpoint KV 接口；租户隔离、TTL、加密、迁移和事件重放要由 Koko 实现 |
| 安全与合规 | 取决于 Koko | Eino 是编排框架，不替代 RBAC、CommandGuard、DLP、审计和数据出境策略 |

## 4. 推荐架构

```text
Vue 3 Agent Drawer
  useTerminalAgent() + @ai-sdk/vue useChat()
  KokoWebSocketChatTransport
                 |
                 | Koko versioned canonical events
                 v
Agent WS / transport mapper -------------------- reconnect by sequence
                 |
                 v
Koko TerminalAgentService                       <- authoritative state
  Session / Run / PlanState / Approval / Audit
  Budget / Policy / Idempotency / EventStore
                 |
                 v
EinoEngineAdapter                                <- replaceable orchestration
  adk.Runner + ChatModelAgent
  Eino event -> Koko domain action/event mapper
                 |
       +---------+----------+
       |                    |
Koko ModelPort       Koko Tool Adapter
provider adapter     fixed tool catalog only
                            |
                    Policy + Approval + Resolver
                            |
                    AgentInputGateway + InputLease
                            |
                    Parser / ACL / Review / srvConn
                            |
                       active PTY
```

边界必须满足：

1. `internal/agent/domain`、`application` 和 `ports` 不导入 Eino 类型；
2. Eino 的 `schema.Message`、`AgentEvent`、checkpoint bytes 只存在于 adapter 内；
3. Koko canonical event 先持久化，再投影到 WebSocket/UI；不能把 Eino event 直接当审计记录；
4. Eino tool 只能调用 Koko application port，不能持有 `srvConn`、资产 secret 或裸 `Room.Receive`；
5. Eino engine 可通过 feature flag 替换或关闭，已有 terminal 数据面不受影响。

建议代码边界：

```text
internal/agent/
  domain/                       # 不依赖 Eino/HTTP/WebSocket/AI SDK
  application/
  ports/
  adapters/
    orchestration/eino/         # Runner、event mapper、checkpoint bridge
    llm/                        # OpenAI Responses / compatible adapters
    active_terminal/
    core/
  tools/                        # Koko 固定 catalog/resolver，不注册 Eino 内置 shell
  transport/ws/                 # Koko event <-> UI projection

ui/src/
  ai/transport/KokoChatTransport.ts
  hooks/useTerminalAgent.ts
  components/AgentDrawer/
  types/agent.ts
```

## 5. Go 后端接入方法

### 5.1 第一阶段只用 `ChatModelAgent`

PoC 从单 Agent 开始，不启用 `DeepAgent`、sub-agent transfer、filesystem middleware 或通用 shell：

```go
agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Name:         "koko-terminal-agent",
    Instruction:  instruction,
    Model:        modelAdapter,
    MaxIterations: 8,
    ToolsConfig: adk.ToolsConfig{
        ToolsNodeConfig: compose.ToolsNodeConfig{
            Tools:               kokoTools,
            ExecuteSequentially: true,
            UnknownToolsHandler: rejectUnknownTool,
            ToolArgumentsHandler: validateCanonicalArguments,
        },
    },
})

runner := adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:           agent,
    EnableStreaming: true,
    CheckPointStore: checkpointStore,
})
```

以上为结构示意，不应直接复制成生产代码。生产配置还必须注入 run budget、模型 timeout、tool timeout、request ID、tenant/session/run context、redactor、callback 和幂等控制。

特别注意：Eino `ToolsNodeConfig.ExecuteSequentially` 默认是 `false`，即一轮多个 tool call 会并行执行。Koko 必须显式设为 `true`；即使如此，`AgentInputGateway/InputLease` 仍是最终串行保证，不能只依赖框架配置。

### 5.2 Koko Tool Adapter

每个对模型暴露的工具都应由 Koko 固定注册：

```text
Eino tool input
  -> 严格 schema decode
  -> Koko ToolDefinition lookup
  -> bind session/run/plan revision/step
  -> capability + policy check
  -> resolver creates ResolvedAction + digest
  -> approval gate if required
  -> InputLease + active PTY readiness
  -> AgentInputGateway.Submit
  -> observe Parser/execution lifecycle
  -> redacted canonical ToolResult
  -> Eino tool result
```

禁止直接采用 Eino `DeepAgent` 的 `Shell` / `StreamingShell` 或 filesystem middleware 操作生产资产。这些接口面向通用 Agent 能力，不会自动经过 JumpServer 的 session binding、CommandGuard、复核、录像和命令记录链路。

### 5.3 PlanState

Koko 已确定 Plan 是版本化领域对象。Eino 的默认方案不能直接替代它：

- `adk/prebuilt/planexecute.defaultPlan` 主要是 `[]string`；
- `plantask` 的 task 带 status/dependency，但默认是 Agent 工具 + Backend 存储语义，不具备 Koko 的 plan revision、step intent digest、resolved action digest、approval subject 和 observation evidence 约束；
- `DeepAgent` 的 todo/task 更偏复杂任务分解与 sub-agent 委派，不等同于单 active PTY 的安全执行计划。

建议保留 Koko `PlanStore` 和 `PlanUpdate` control action。可以通过 Eino middleware/customized output 把模型提出的结构化计划交给 Koko validator；验证成功后由 Koko 发布 plan revision，再把当前有效 Plan 摘要注入下一次模型输入。不能从 assistant Markdown 反向解析计划。

### 5.4 审批、中断与恢复

Eino HITL 可用作“暂停编排并在批准后继续”的机制：

1. tool adapter 完成 schema/policy/resolver 后发现需要审批；
2. Koko 创建 canonical `ApprovalRequested`，包含 exact action 和全部 digest；
3. adapter 触发 stateful interrupt，checkpoint ID 使用不可猜测的内部 ID；
4. UI 只提交 `approval_id + decision`，服务端重新认证并核对 session、权限、expiry、revision 和 digest；
5. Koko 先记录审批决定；仅在有效时调用 Eino resume，并传入最小 resume data；
6. tool 恢复后再次执行 freshness/policy/lease 检查，再提交 active PTY。

Eino checkpoint 只用于恢复框架执行栈，不是审批凭证，也不是 Agent 事实源。`CheckPointStore` 只有 `Get/Set`，可选 `Delete`；Koko 实现必须额外保证：

- tenant/session/run scope；
- 加密、TTL、大小和并发版本限制；
- checkpoint 与 canonical event/approval 的关联；
- 消费或结束后的删除；
- Eino 升级时的 gob 兼容与迁移测试。

源码中已经存在历史 checkpoint gob 兼容预处理，说明 checkpoint schema 升级是实际风险。因此必须固定 Eino 精确版本，不能用 branch/伪版本无审查升级。

### 5.5 Event 映射与持久化顺序

建议 Eino adapter 将事件映射为 Koko 领域事件：

| Eino 输出 | Koko canonical event | UI 投影 |
| --- | --- | --- |
| assistant message stream | `AssistantTextDelta` | `text-start/delta/end` |
| assistant tool call | `ToolCallProposed` | `tool-input-*` |
| tool result | `ToolExecutionObserved` | `tool-output-available/error/denied` |
| interrupt | `ApprovalRequested` 或 `RunPaused` | `tool-approval-request` + domain data part |
| runner error | `RunFailed` | `error` + finish |
| runner finish | `RunCompleted` | `finish` |

顺序必须是：

```text
Eino event -> validate/map -> append Koko EventStore -> assign sequence
           -> update projections -> WebSocket single writer -> Vue
```

如果 WebSocket 断开，客户端按 `run_id + last_sequence` 恢复 Koko event；不能通过重新调用 `runner.Query` 恢复，否则可能重放 tool 副作用。

## 6. Vue 与 `ai` / `@ai-sdk/vue` 接入

### 6.1 版本与兼容性

2026-07-13 从 npm registry 观察到：

- `ai 7.0.22`，peer dependency 为 `zod ^3.25.76 || ^4.1.8`；
- `@ai-sdk/vue 4.0.22`，peer dependency 为 `vue ^3.3.4`；
- Koko 当前 Vue `^3.5.16` 满足要求，但项目尚未安装 `ai`、`@ai-sdk/vue` 和 `zod`。

正式引入时应锁定经 contract test 验证的精确版本，不使用 `^` 或 `latest`。当前 SSOT 中的研究基线是 `ai 7.0.20` / `@ai-sdk/vue 4.0.20`，与 registry 最新版本存在两个 patch 的差异；PoC 应先选定一组版本并生成 lockfile diff、bundle size 和协议 golden test，再决定是否升级基线。

### 6.2 使用自定义 `ChatTransport`

`@ai-sdk/vue` 的 `useChat()` 接收 `ChatInit`，底层 `ChatTransport` 暴露 `sendMessages()` 和 `reconnectToStream()`，返回 `ReadableStream<UIMessageChunk>`。因此可以保留独立 Agent WebSocket：

```ts
const chat = useChat(() => ({
  id: agentSessionId.value,
  transport: new KokoWebSocketChatTransport({
    terminalSessionId: terminalSessionId.value,
  }),
}));
```

`KokoWebSocketChatTransport` 的职责是：

- 建立/复用 `/koko/ws/agent`；
- 给请求附加 terminal session binding，但不信任浏览器自报身份；
- 将 Koko canonical event 映射为 `UIMessageChunk`；
- 维护 `run_id/sequence` 并实现 `reconnectToStream()`；
- 将 `AbortSignal` 映射为 Koko cancel 请求；
- 严格区分 cancel、take over、interrupt remote process 和 approval response；
- socket 断开时结束本地 stream，但不把服务端 run 判定为失败或重新提交。

### 6.3 UI 消息不是事实源

AI SDK UI protocol 已有 text、reasoning、tool input/output、approval、error、start/finish 等 chunk，可复用其消息 reducer。Koko 专属的 Plan revision、step timeline、execution state、lease、action digest 和 takeover 应使用 typed `data-*` part 或旁路 Pinia projection，不应塞入 assistant 文本。

推荐状态分工：

| 状态 | 权威位置 |
| --- | --- |
| conversation message/tool 展示 | `useChat.messages` 投影 |
| plan revision/step/evidence | Koko EventStore + Vue PlanStore 投影 |
| approval 决定 | Koko 服务端；UI 只发意图 |
| active execution/lease | Koko owner node |
| reconnect cursor | Koko sequence + transport 本地 cursor |
| terminal bytes | 现有 terminal WebSocket/xterm，不进入 Agent WS |

### 6.4 UI 落点

当前 `ui/src/components/Drawer/index.vue` 已有右侧 Drawer 和 tab，可以新增 Agent tab，但建议把 Agent 组件拆分为独立目录，避免在现有 Drawer 文件内堆积运行状态。首版界面至少展示：

- conversation 与 streaming status；
- structured Plan timeline；
- tool input preview、风险和审批卡；
- running/unknown/failed/unsupported 等真实执行状态；
- Stop generation、Interrupt process、Take over 三个语义不同的操作；
- reconnect/recovered 标识和最后事件 sequence。

## 7. Provider 策略

有两条可行路线：

### 路线 A：Koko Provider Adapter 实现 Eino model interface

保留 SSOT 已接受的 Provider 决策：OpenAI 官方服务使用 `openai-go/v3` Responses API，兼容端点使用独立 adapter；再在 Eino adapter 内实现其 `model.ToolCallingChatModel` 接口。

优点是 Provider 行为、`store:false`、strict schema、usage、reasoning 和兼容差异仍由 Koko 控制。缺点是要维护 Eino message 与 Koko provider DTO 的双向映射。

### 路线 B：采用 `eino-ext` model 实现

接入速度可能更快，但在采用前必须验证：

- 是否实际使用目标 Provider 所需的 Responses/Chat Completions 协议；
- `store:false`、tool schema strict、reasoning、usage、finish reason、cancel 和错误映射；
- 自定义 base URL/header/proxy/TLS 的组织策略；
- provider opaque state 与 Koko canonical conversation 的边界。

在没有完成以上 contract test 前，推荐路线 A。Eino core 不应反向改变已接受的 Provider 安全决策。

## 8. 主要风险与控制

| 风险 | 后果 | 控制 |
| --- | --- | --- |
| Eino tool 默认并行 | 同一 PTY 输入交错或并发副作用 | `ExecuteSequentially=true` + Koko InputLease 双重保证 |
| 把 Eino checkpoint 当权威状态 | 升级不可读、审批/审计关联丢失 | Koko canonical store 为权威；checkpoint 只恢复 engine |
| 直接使用通用 shell tool | 绕过 ACL/review/录像/active session | 只注册 Koko fixed tools；所有命令走 InputGateway |
| 默认 Plan 语义过弱 | 无 step digest/evidence，无法安全恢复 | Koko PlanState 独立实现并版本化 |
| Eino event 直接推 UI | 无持久化 sequence，重连可能重做 | 先落 Koko event，再单 writer 推送 |
| UI approval 被当授权 | 客户端可伪造/重放 | 服务端认证、policy/digest/freshness 再校验 |
| 框架/AI SDK 快速升级 | checkpoint 或 stream protocol 破坏 | 固定精确版本、golden/contract/migration tests |
| 引入 Eino 与 sonic 依赖升级 | 影响 Koko 现有依赖树 | PoC 单独检查 `go mod graph`、全量测试和性能基线 |
| Agent 故障影响 terminal | 核心终端不可用 | 独立 WS、独立 context、feature flag、无权关闭 srvConn |

## 9. PoC 实施步骤

### P0：依赖和契约验证

1. 在隔离 branch 固定 `eino v0.9.12`；记录 `go.mod/go.sum` 变化和 sonic 等传递依赖升级。
2. 实现 fake model + 两个无副作用工具，不连接真实 PTY。
3. 验证 stream、cancel、MaxIterations、未知工具、顺序执行、timeout 和 callback。
4. 为 Koko canonical event 与 `UIMessageChunk` 建 golden fixtures。

### P1：只读副驾驶

1. 新建 Eino engine adapter 和独立 Agent WS。
2. Vue 新建 custom `ChatTransport`、`useTerminalAgent()` 和 Drawer tab。
3. 只开放 `inspect_ssh_session`、`propose_command`、`explain_output` 等无远端副作用能力。
4. 验证断线按 sequence 恢复，不重新提交模型请求。

### P2：Plan 与 HITL

1. 接入 Koko canonical PlanState/PlanUpdate validator，不使用 Eino default plan 作权威状态。
2. 实现加密、TTL、scoped checkpoint store。
3. 用 fake approval 验证 interrupt/resume、拒绝、过期、digest stale 和单次消费。
4. 做 checkpoint 跨进程恢复及 Eino patch 升级兼容测试。

### P3：active PTY 只读 Tool

必须等 SSOT Phase 3 的 BindingRegistry、AgentInputGateway、InputLease、ResolverRegistry 和 CommandGuard 条件满足后再做。先开放一个固定只读 tool，进行 ACL/review、共享会话、用户 takeover、partial write、unknown completion 和重连故障注入测试。

## 10. PoC 通过标准

满足以下条件才建议把 Eino 从 `Proposed` 提升为正式实现选择：

1. 100 次断线/恢复测试中没有重复 tool execution；
2. 模型一次提出多个 tool call 时，同一 terminal 的执行严格串行；
3. approval 拒绝、过期、digest stale、权限变化均 fail closed；
4. kill Koko worker 后能从 canonical event + checkpoint 恢复展示，不能重放已提交命令；
5. Eino 异常、panic、stream error 和 callback error 不影响 terminal WebSocket；
6. AI SDK chunk golden test 覆盖 text/tool/approval/error/finish/reconnect；
7. 固定版本后全量 Go/Vue 构建和现有测试通过；
8. CPU、内存、goroutine、首 token 延迟和 bundle size 满足约定预算；
9. 安全 Review 确认 Eino adapter 没有绕过 Koko policy、audit 和 active PTY gateway 的路径。

## 11. 最终建议

建议批准一个“单 Agent、只读、无真实 PTY 写入”的 Eino PoC，采用 `v0.9.12` 稳定 tag，并把 Eino 固定在 `adapters/orchestration/eino` 内。

PoC 成功后，Eino 可以减少 Koko 在 Agent loop、stream、interrupt/resume 和 middleware 上的自研工作；但它不减少 PlanState、安全执行、审批、事件持久化和多实例恢复的核心工作量。若 PoC 发现 checkpoint 迁移、Provider 映射或事件协议适配成本超过收益，应能在不改 domain/application/UI canonical contract 的前提下换回自研 engine。

## 12. 证据与参考

- Eino 本地源码：`/opt/codes/eino`
- Eino README：<https://github.com/cloudwego/eino>
- Eino ADK：<https://github.com/cloudwego/eino/tree/main/adk>
- Eino compose：<https://github.com/cloudwego/eino/tree/main/compose>
- Eino releases：<https://github.com/cloudwego/eino/releases>
- Eino 扩展组件：<https://github.com/cloudwego/eino-ext>
- AI SDK Vue：<https://ai-sdk.dev/docs/reference/ai-sdk-ui/use-chat>
- AI SDK stream protocol：<https://ai-sdk.dev/docs/ai-sdk-ui/stream-protocol>
- Koko 正式设计：[ssh-agent-design.md](./ssh-agent-design.md)
- Koko 研究总结：[terminal-ssh-agent-summary.md](./terminal-ssh-agent-summary.md)
- Koko 旧 Chat：`pkg/httpd/chat.go`、`pkg/srvconn/conn_openai.go`
- Koko active PTY：`pkg/proxy/switch.go`、`pkg/proxy/parser.go`
- Koko Vue：`ui/package.json`、`ui/src/components/Drawer/index.vue`、`ui/src/hooks/useTerminalSocket.ts`
