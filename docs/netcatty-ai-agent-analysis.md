# Netcatty AI / Agent 代码分析与借鉴总结

> 状态：基于本地仓库 `/opt/codes/Netcatty` 的源码阅读记录
>
> 更新时间：2026-07-12
>
> 目的：为 Koko Terminal SSH Agent 的设计提供可追溯的实现参考；本文不把 Netcatty 的本地 Electron 信任模型等同于 Koko 的多租户、审计型服务端安全边界。

## 1. 总览

Netcatty 是一个 Electron + React 的桌面终端客户端。它的 AI 能力以 **Catty Agent** 为产品入口：用户可以在终端、工作区或全局范围内对话，让模型读取受限上下文、调用终端/SFTP/Vault 等工具，或者切换到由本机 CLI/SDK 驱动的外部 Agent。

从代码看，它不是“模型直连终端”，而是由以下层次组成：

```text
AI Chat Side Panel
       |
useAIChatStreaming（UI 状态）
       |
AgentRuntime（turn 串行、事件、停止、临时上下文）
       +-- CattyTurnDriver（Vercel AI SDK streamText）
       +-- ExternalSdkTurnDriver（Codex/Claude/Copilot/... SDK）
       |
Capability Catalog -> Catty tools / MCP / CLI / RPC surfaces
       |
Electron bridge / terminal worker / SFTP / vault / port forwarding
```

核心思想是：**turn 生命周期由运行时统一管理；能力元数据由 catalog 统一声明；模型与外部 Agent 的流都适配为统一事件；写操作通过 permission mode 和 approval gate 管理。**

## 2. 已实现的 AI 产品能力

| 能力 | 实现情况 | 关键源码 |
| --- | --- | --- |
| 内置聊天 Agent（Catty） | 通过 Vercel AI SDK `streamText` 调用配置的模型，支持流式文本、thinking、tool call、附件、会话历史 | `infrastructure/ai/harness/turnDrivers/cattyTurnDriver.ts`、`cattyStreamProcessor.ts` |
| 多模型提供商 | 配置 OpenAI、Anthropic、Google、Ollama、OpenRouter、Qwen、DeepSeek、Kimi、智谱、豆包、自定义 endpoint；按 OpenAI/Anthropic/Google 三类 wire style 建 client | `infrastructure/ai/types.ts`、`infrastructure/ai/sdk/providers.ts` |
| 外部 Agent | 管理/发现 Codex、Claude、Copilot、Cursor、CodeBuddy、OpenCode 等本机 Agent；通过 SDK backend 运行并转换流事件 | `infrastructure/ai/managedAgents.ts`、`sdkAgentAdapter.ts`、`harness/turnDrivers/externalSdkTurnDriver.ts` |
| 范围化会话 | AI session 绑定 terminal、workspace 或 global scope；上下文中携带可访问 terminal sessions、host chain 与 active port forwards | `infrastructure/ai/types.ts`、`components/AIChatSidePanel.tsx`、`application/state/aiStateSnapshots.ts` |
| 工具调用 | 终端短命令/长任务、SFTP、Vault、端口转发、附件、会话控制、web search、URL fetch、terminal context read 等 | `electron/capabilities/catalog/*.cjs`、`infrastructure/ai/shared/toolExecutors.ts` |
| Web 检索 | 接入 Tavily、Exa、Bocha、智谱、SearXNG；未完成配置时不向 Catty 暴露 `web_search` | `infrastructure/ai/types.ts`、`shared/webSearchProviders.ts`、`harness/capabilityTools.ts` |
| 用户 Skills | 从用户选择的 skill slug 生成上下文，并传给 Catty/外部 SDK Agent | `components/ai/userSkillsState.ts`、`components/ai/hooks/aiChatStreamingSupport.ts` |
| 对话操作 | session 历史、scope 过滤、草稿、导出、会话清理、外部 Agent session ID 续接 | `components/AIChatSessionHistoryDrawer.tsx`、`infrastructure/ai/conversationExport.ts`、`components/ai/externalAgentHistory.ts` |

README 将产品定位为运维伙伴，宣传诊断、日志/资源检查、多主机协作和复杂操作；实际授权边界仍由 capability、permission mode 和 bridge 执行路径决定，不能只依据营销描述判断其安全性。

## 3. Agent Runtime：把 UI 与执行生命周期分离

### 3.1 单一运行时与 Driver 抽象

`AgentRuntime` 维护 backend 到 `TurnDriver` 的映射，并在 `globalAgentRuntime.ts` 中注册两类 driver：

- `catty`：内置模型 + Vercel AI SDK；
- `external-sdk`：外部 CLI/SDK Agent。

`useAIChatStreaming` 只组织 UI 所需输入、调用 `runTurn`/`stopTurn` 和更新显示，不自行实现一套模型 loop。`AgentRuntime.runTurn` 对相同 `chatSessionId` 等待已有 turn 结束，因此同一聊天会话不会同时运行两个 driver；它还为 session 持有 `ToolOutputStore`、为每个 turn 新建 `ToolResultDedup`，并把 driver 事件写入 trace 后 fan-out 给订阅者。

这比把 abort controller、tool state、模型流和界面状态分散在多个 React hook 中更稳定。尤其是 Catty 与外部 Agent 的前端消息格式不同，统一 runtime 避免两条路径的停止与审计行为继续漂移。

### 3.2 统一事件协议

`harness/types.ts` 定义了统一 `AgentEvent`，包括：

- turn：`turn_start`、`turn_end`；
- 模型：`model_call_start`、`model_delta`、`reasoning_delta`、`step_end`、`usage`、`performance`；
- 工具：`tool_call`、`tool_result`；
- 审批：`approval_requested`、`approval_resolved`；
- 上下文：`compaction_start`、`compaction`；
- 异常：`error`。

外部 SDK 的原始回调由 `agentEventAdapter.ts` 映射到这些事件；Catty stream processor 则从 AI SDK 生命周期回调生成模型调用、step usage 和 performance 事件。此协议同时服务 UI、trace 和 session state 更新，值得借鉴。

局限是 trace store 是进程内调试存储（默认最多 2,000 events），不是防篡改审计日志；Koko 必须将 canonical event 持久化并与用户、资产、策略、命令记录关联。

### 3.3 停止语义

`stopAgentTurn()` 是 UI、slash stop 和 MCP 的公共停止入口。它负责：取消下游 bridge/Agent、清理指定 chat session 的 pending approvals、写 `turn_end` trace。`AgentRuntime.stopTurn()` 先要求 active driver abort，再调用该入口并等待 active promise 收敛。

这是一个正确的收敛方向。但 Netcatty 的运行时状态主要在 renderer 内存；Koko 还需将 cancel 状态、approval 决议和 executor 结果持久化，以处理服务重启和多实例。

## 4. 内置 Catty 的模型与上下文链路

### 4.1 提供商适配

`ProviderConfig` 将 provider identity 与 wire style 分离：`providerId` 用于显示/路由，`style` 决定请求协议家族，默认落到 OpenAI、Anthropic 或 Google。配置包含 API key、base URL、自定义 headers、TLS 跳过开关、默认模型、context window 覆盖/发现值和常用采样参数。

Catty driver 以 provider 配置创建模型、构建 system prompt、生成 AI SDK message 和 tools，然后调用 `streamText`。模型配置、provider continuation、OpenAI assistant fields 的保留被放在 adapter/message builder 层，避免这些厂商细节直接污染通用 chat message。

可借鉴点是 protocol family 与 provider display identity 分离；不应照搬的是让浏览器直接成为所有 provider credential 与策略的权威位置。Koko 应把 key、数据出境策略和请求日志置于服务端 adapter。

### 4.2 System Prompt 与 scope

`cattyAgent/systemPrompt.ts` 以当前 scope、host/session 列表、权限模式、web search 是否可用和用户 skill 生成 prompt。它包含工具使用规则、附件导入规则，以及网络设备/serial session 不能使用 shell 管道、重定向、环境变量等专门约束。

这种“根据资产 profile 注入操作限制”的做法值得保留。但 prompt 只是给模型的提示，不能替代工具层的 scope 校验、命令策略或审批。

### 4.3 上下文预算与压缩

相关模块：`contextBudget.ts`、`tokenEstimator.ts`、`staleContextPruner.ts`、`compactionPruner.ts`、`contextManager.ts`、`contextCompaction.ts`、`cattyRuntime.ts`。

其策略分三层：

1. 根据 provider/model context window、最大输出和 reserve 估算预算；
2. 每个 step 做确定性处理：裁剪陈旧工具上下文、typed compression、保留输出 handle 提示；
3. turn 前或发生请求过大（413）时可调用模型总结旧消息，保留最近消息，并重新注入 session state/用户目标等事实。

`CompactionTrace` 记录触发原因、前后 token/消息数、保留尾部数、是否 typed compression/LLM summary/413 fallback 和估算器类型。这使上下文变更可诊断，而不是静默截断。

Koko 可采用同样的“确定性裁剪优先、摘要兜底、413 专项降级、trace 可观测”原则；摘要不能被当作授权或执行事实，且敏感终端输出必须先按 Koko 数据策略脱敏。

### 4.4 SessionState 与 continuation

`sessionState.ts` 根据用户目标、assistant 文本和 tool result 提取/保留跨 turn 的 session facts，压缩后再注入上下文。`providerContinuation.ts` 处理 provider 专属的隐藏/推理相关 message fields，避免 provider 切换时错误重放另一厂商的内部上下文。

Koko 应保留“厂商 opaque continuation 与领域事实分离”的设计：领域事实来自工具和用户，provider opaque state 仅限相同 adapter、加密且短期保存。

## 5. Capability Catalog：工具面的一处定义，多处生成

Netcatty 的高价值设计是 `electron/capabilities/catalog/`。每个 capability 声明：

- 稳定 ID、domain、状态、描述；
- policy：`write`、`sensitiveRead`、`longRunning`、`requiresChatSession`、是否绕过 observer/approval/chat cancel；
- surface：Catty、内建 RPC、公开 RPC/MCP、CLI、未来 global agent 等的名称/方法。

`toolSurfaces.cjs` 根据 catalog 生成 Catty tool specs、global agent tool specs 和 MCP registry；CLI 和 RPC dispatch 也引用同一目录。`AGENTS.md` 说明当前 CLI 可用 catalog command 为 30 个，harness 本地工具不暴露给 MCP。

主要能力域如下：

| 域 | 代表能力 | 写入/敏感特点 |
| --- | --- | --- |
| Terminal | execute、start、poll、stop | execute/start 是写且需审批；poll/stop 有专门例外 |
| SFTP | list/read/stat/home、write/upload/download、mkdir/delete/rename/chmod | read/list 标为 sensitive read；写和传输均为 write |
| Vault | host、notes、snippets、scripts 与运行管理 | 密钥/private key 不经 vault Agent bridge 返回 |
| Port forwarding | rules/tunnels list、start、stop | start/stop 为写和长任务 |
| Meta | environment、status、attachments、session cancel/resume/get | 会话控制与附件读取 |
| Harness（仅 Catty） | output read、workspace/session info、bounded terminal context、web search、URL fetch | renderer 本地，不能经 MCP/CLI 调用 |

这比为 Chat、MCP、CLI 分别维护工具 schema 更不容易漂移。Koko 应以同样的 catalog 为单一事实源，但 capability policy 还必须纳入 Core RBAC、资产 ACL、CommandGuard、工单/复核和审计分类，不能只使用布尔 `write`。

## 6. 工具执行、范围与并发

### 6.1 工具 context 不依赖闭包

Catty 使用 `toolsContext`：每个工具在 `execute(args, { context })` 中从 typed context 取 bridge、动态 executor context、输出 store、权限模式等依赖，而不是捕获渲染时的闭包。工具执行前通过 `validateSessionScope()` 检查 `sessionId` 是否在当前 scope；observer mode 会拒绝终端执行。

这种注入方式适合避免切换工作区、终端重连或 React state 变化后，工具仍引用过期 session。Koko 的等价物应是每个 tool call 从 Repository/Core 重新加载 subject、asset、session 与权限，而不是使用浏览器快照。

### 6.2 同 session 的执行队列

AI SDK 可能使用 `Promise.all` 并行执行一轮中的多个 tool call。Netcatty 的 `sessionExecutionQueue.ts` 因此以 session key 创建 reservation slot：每个调用先同步占位，可以并行等待 approval；真正调用 PTY 前按预订顺序等待 slot，结束、拒绝、错误时在 `finally` release。底层 main process mutex 仍作为兜底。

这是避免同一 PTY 命令冲突的关键实现。Koko 应保留这一模式，但执行 key 不能只看 terminal session：对于独立 SSH/K8s/数据库 executor，还需要按资产、账号、资源锁或变更目标定义更严格的串行策略。

### 6.3 工具输出 handle 与去重

`ToolOutputStore` 按 chat session 保存完整输出，并只将 preview/截断结果给模型。`tool_output_read` 可以按 head/tail/full 和 max chars 读取；`toolResultFitting.ts` 会在结果中告诉模型 output 被截断及 handle ID。大 terminal context 和 SFTP 文件读取使用此机制，避免重复把大文本放入每轮 prompt。

`ToolResultDedup` 对同 turn 内相同读工具结果记录 fingerprint，命中时返回 cached notice 而非再次读取。二者都减轻 token 与执行负担。

Netcatty 实现中的完整内容仍是 renderer 进程内 map，并随 chat session 删除而清理。Koko 不应把敏感数据放入无保护的进程内长期 map：应使用有 session/run scope、TTL、大小上限、加密/脱敏状态、访问审计的持久化 `ToolOutputRef`，并禁止跨会话读取。

## 7. 权限与审批

### 7.1 三种模式

`AIPermissionMode` 包含：

- `observer`：阻止写工具；
- `confirm`：写工具弹出确认；
- `auto`：按当前设置尽量自动执行。

此外存在 command blocklist、命令 timeout、host permission、可持久化 permission grants。catalog 中 SFTP 写/传输、port forward start、vault host notes set 等 capability 显式标记为需确认；harness 的只读工具一般 bypass approval。

### 7.2 审批 Gate

`approvalGate.ts` 维护按 `toolCallId` 关联的 pending approval。审批请求通知 UI；用户 approve/deny 后 resolve Promise。其特性包括：

- 默认五分钟超时自动拒绝；
- 以 `chatSessionId` 过滤，停止一个会话不会误清理其他会话；
- UI unmount/remount 时可 `replayPendingApprovals`；
- Catty 与外部 MCP 的审批均映射到同一 UI store；
- 可将已批准的工具/命令规则保存为 permission grants。

这解决了前端审批容易悬挂的问题。Koko 需要进一步强化：approval 应写入 Repository，绑定执行 action digest、资产/账号、policy revision、run/tool call 和过期时间；任何参数或策略变化都必须使旧审批失效。浏览器内 Promise 只能作为 UI 等待机制，不能是授权事实源。

### 7.3 命令 grant 的谨慎处理

`shellCommandGrant.ts` 对 shell 命令做 tokenization、segment 拆分和可授予 pattern 提取，用于“always allow”一类权限规则。它试图处理引号、连接符等场景。

该功能适合本地个人工具的便利性，但对 Koko 风险较高：复杂 shell 语义、变量展开、解释器差异、网络设备 CLI 和数据库语法都使 pattern 授权易被绕过。Koko 应优先使用语义化 Tool + 确定性 resolver；通用 shell grant 只能作为严格限制、强审计且默认短期的例外。

## 8. 外部 Agent、MCP 与 CLI 集成

Netcatty 同时支持内置 Catty 和外部 Agent。外部 driver 会将当前终端 sessions、附件、用户 skills、历史与目标 session 同步给 main-process bridge，调用 SDK Agent，并将 text/thinking/tool/status/session ID callbacks 映射为统一 UI/trace 状态。

工具还可通过以下面向外部集成的渠道提供：

```text
Capability catalog
   +-- Catty：AI SDK tools（含 renderer-local harness tools）
   +-- MCP stdio：netcatty-mcp-server.cjs
   +-- CLI：netcatty-tool-cli.cjs
   +-- RPC：mcpServerBridge.cjs / capabilityRpcDispatch.cjs
```

优势是让外部 Agent 与内置 Agent 使用大部分相同 capability 元数据。需要注意：Netcatty 的外部 Agent 是用户机器上的本地集成面，不应被误认为是稳定的公共 SaaS API。Koko 若开放 MCP，应将其视为独立的服务端安全边界：认证、租户、RBAC、审批、速率、审计和版本兼容必须单独设计。

## 9. UI、可观测性与测试

UI 由 `AIChatSidePanel`、`AIChatPanelContent`、history drawer、ChatInput、ChatMessageList、ToolCallGroup、ThinkingBlock、approval host 等 React 组件组成。工具调用、thinking、状态文字、审批卡和外部 Agent 结果均以 ChatMessage 的附加字段呈现；Netcatty 也维护 conversation export 与诊断视图。

测试覆盖相对深入：runtime lifecycle/stop、approval、session execution queue、tool output store、dedup、context budget/compaction、capability tool、provider continuation、stream mapping、外部 Agent history，以及 Catty system prompt 等均有单元测试。catalog 还带生成物漂移和完整性测试。

值得借鉴的是把测试重点放在 **跨层契约**：同一 tool 在不同 surface 的 schema/策略一致性、stream event 映射、取消、上下文裁剪、approval replay 和并发序列。Koko 应再增加持久化事件重放、权限实时复核、审计落库失败处理和多实例 lease 的集成测试。

## 10. 对 Koko 的借鉴结论

### 应直接吸收的设计原则

1. 建立 `AgentRuntime + TurnDriver + canonical event`，让 Vue/UI 只显示状态。
2. 用 capability catalog 统一工具 ID、schema、风险、surface 与生成检查，避免 Agent/MCP/CLI 各自漂移。
3. 将同 session/资产工具调用的“审批并行、实际执行串行”落实为 reservation queue，并保留 executor 互斥兜底。
4. 对长输出使用 handle + preview + 定量 read；对陈旧和重复 read 做裁剪/去重。
5. 采用 pre-turn、step、413 retry 分层的上下文预算策略，并记录 compaction trace。
6. 统一所有停止路径；approval 支持超时拒绝、会话范围清理和重连重放。
7. 将模型 provider protocol family 与领域接口隔离；provider opaque continuation 不能污染领域会话事实。
8. 将终端/工作区 scope 显式传给工具，并在每次调用时重新校验。

### 必须改造后才能用于 Koko 的部分

| Netcatty 做法 | Koko 所需改造 |
| --- | --- |
| renderer 内存维护 runtime、trace、pending approval、tool output | Repository 持久化 canonical event、run、approval、ToolOutputRef；浏览器仅为投影 |
| Electron bridge 调用本机终端/文件能力 | 经 Koko Core、ConnectToken、资产/账号 ACL、CommandGuard、复核和 Recorder 执行 |
| `observer/confirm/auto` 三档模式 | 结合风险 R0-R4、命令 ACL、工单、实时 RBAC 和资产策略，模型无权决定风险 |
| 通用 shell command grant | 优先语义化 Tool 与确定性 command resolver；通用命令仅受限逃生通道 |
| 进程内 ToolOutputStore 保存 full content | 带脱敏、加密、TTL、scope、读取上限和访问审计的受控引用 |
| 本地 MCP/CLI 集成面 | 独立服务端 API 合同与认证授权；不能默认信任调用方环境 |
| 2000 条内存 trace | 不可篡改/可查询的审计事件，关联 run、tool、command record 与 verification |

## 11. 源码索引

| 主题 | Netcatty 源码 |
| --- | --- |
| AI 架构说明 | `/opt/codes/Netcatty/AGENTS.md` |
| 产品入口/UI | `/opt/codes/Netcatty/components/AIChatSidePanel.tsx`、`components/ai/hooks/useAIChatStreaming.ts` |
| Runtime/事件/停止 | `/opt/codes/Netcatty/infrastructure/ai/harness/agentRuntime.ts`、`types.ts`、`agentStop.ts` |
| Catty Driver | `/opt/codes/Netcatty/infrastructure/ai/harness/turnDrivers/cattyTurnDriver.ts`、`cattyStreamProcessor.ts` |
| 外部 Agent Driver | `/opt/codes/Netcatty/infrastructure/ai/harness/turnDrivers/externalSdkTurnDriver.ts`、`sdkAgentAdapter.ts` |
| 上下文 | `/opt/codes/Netcatty/infrastructure/ai/harness/contextManager.ts`、`contextBudget.ts`、`staleContextPruner.ts`、`../contextCompaction.ts` |
| 工具与输出 | `/opt/codes/Netcatty/infrastructure/ai/harness/capabilityTools.ts`、`toolOutputStore.ts`、`toolResultDedup.ts` |
| 执行队列与审批 | `/opt/codes/Netcatty/infrastructure/ai/shared/sessionExecutionQueue.ts`、`approvalGate.ts`、`shellCommandGrant.ts` |
| Capability catalog | `/opt/codes/Netcatty/electron/capabilities/catalog/`、`codegen/toolSurfaces.cjs` |
| MCP/CLI/RPC | `/opt/codes/Netcatty/electron/mcp/`、`electron/cli/netcatty-tool-cli.cjs`、`electron/bridges/mcpServerBridge.cjs` |
| Provider/设置类型 | `/opt/codes/Netcatty/infrastructure/ai/types.ts`、`sdk/providers.ts` |

