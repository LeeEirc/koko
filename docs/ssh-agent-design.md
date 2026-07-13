# Koko SSH Terminal Agent 设计稿

> 文档角色：Terminal Agent 的规范性设计与决策单一事实源（SSOT）
>
> 状态：Draft / 可反复 Review
>
> 当前版本：`0.3.0`
>
> 更新时间：2026-07-13
>
> 研究与证据附录：[terminal-ssh-agent-summary.md](./terminal-ssh-agent-summary.md)

## 0. 文档维护与 Review 规则

本文描述“最终准备实现什么、为什么这样选择、哪些事项仍待决定”。研究过程、竞品源码细节、实验代码和长篇论证保留在 `terminal-ssh-agent-summary.md`；发生冲突时，以本文最新已接受决策为准。

本文使用以下规范词：

- **必须（MUST）**：安全、审计或架构不变量，不满足不得上线；
- **应该（SHOULD）**：默认实现，偏离时必须在 Review 中记录原因；
- **可以（MAY）**：可选能力，不构成首版依赖。

每次 Review 或引入新的参考实现时，必须同步完成：

1. 更新“决策登记”，注明 `Accepted/Proposed/Superseded/Rejected`；
2. 修改受影响的规范章节，而不是只在文末追加说明；
3. 更新“开放问题”和“修订记录”；
4. 在研究附录保留源码基线、实验结果和详细证据；
5. 如结论改变执行、安全或数据边界，提升 minor 版本；破坏协议兼容时提升 major 版本。

建议 Review 顺序：产品边界 → active PTY → 安全/审批 → 运行时/恢复 → 技术栈 → 补全 → UI → 实施与测试。

决策 ID 一经分配不得复用；被替代的决策保留在登记表中并标记 `Superseded by D-xxx`。Review 意见应引用决策 ID 或章节号，避免只留下无法追踪的段落评论。

### Review 记录

| 轮次 | 日期 | Review 范围 | 结果 | 后续动作 |
| --- | --- | --- | --- | --- |
| R0 | 2026-07-13 | 从研究总结提炼初稿 | Pending review | 评审 D-017/D-018 和 Phase 3 前置条件 |
| R1 | 2026-07-13 | Tool 范围 | Accepted | 当前设计只保留 SSH/Linux terminal tools；非 SSH profile 另立设计 |
| R2 | 2026-07-13 | 跨 Linux ResolvedAction 与 digest | Accepted | 用会话级执行画像、版本化 resolver、CommandIR 和 digest chain 约束解析、审批与实际 PTY 字节 |

## 1. 决策摘要

| ID | 决策 | 状态 |
| --- | --- | --- |
| D-001 | Terminal Agent 附着于当前真实 terminal session，不是后台 SSH 自动化器 | Accepted |
| D-002 | 所有命令型 tool call 默认且唯一通过当前 active PTY 执行 | Accepted |
| D-003 | Agent 命令进入与人工输入相同的 Parser、ACL/复核、srvConn、录像和命令记录链路 | Accepted |
| D-004 | ToolRouter 不得直接写 `srvConn`，也不得裸调用 `Room.Receive` 后等待 PS1 | Accepted |
| D-005 | 后端 Agent Runtime、tool loop、provider adapter 和安全策略使用 Go | Accepted |
| D-006 | 前端继续使用 Vue 3/TypeScript/xterm；使用 `@ai-sdk/vue`/`ai`，不引入 React runtime | Accepted |
| D-007 | OpenAI 官方服务使用 `openai-go/v3` Responses API；兼容端点使用独立 adapter | Accepted |
| D-008 | Agent WebSocket、terminal byte stream、completion 请求使用相互隔离的传输 | Accepted |
| D-009 | 补全采用 history/static/metadata/可选 LSP/AI 瀑布；AI 是慢路径和最后 fallback | Accepted |
| D-010 | 补全 metadata 不通过 active PTY 偷跑隐藏 generator | Accepted |
| D-011 | 同一 active terminal 的命令 tool 严格串行，并受 `InputLease` 控制 | Accepted |
| D-012 | PTY ToolResult 允许 `running/unknown` 和 `exit_code=nil`，不得伪造 stdout/stderr/成功 | Accepted |
| D-013 | 模型只提出 tool request；风险、权限、审批和执行由 Koko/Core 决定 | Accepted |
| D-014 | 后台续跑、多资产并行和定时任务属于未来显式 Task Agent，不是自动 fallback | Accepted |
| D-015 | Shell 补全首版使用 Go 原生 `mvdan/sh`；LSP 仅为可选 Provider，不部署 Node sidecar | Accepted |
| D-016 | Warp 等 AGPL 源码只用于设计研究；默认独立实现，不复制非平凡实现 | Accepted |
| D-017 | 跨 Koko 节点的 durable submit 采用内部 RPC 还是可靠 Redis 机制 | Proposed / 待压测 |
| D-018 | shell integration 的具体 hook/控制序列和支持矩阵 | Proposed / 待原型 |
| D-019 | 当前 Tool Registry 只设计 SSH/Linux terminal tools，不包含 Telnet、K8s、数据库或 SFTP tools | Accepted |
| D-020 | ToolDefinition、strict Schema、resolver 和风险基线由后端固定并版本化；模型只能选择 Tool 和填写参数 | Accepted |
| D-021 | 首批远端命令 Tool 为 system/disk/process/port/service/有限日志检查 | Proposed / 待 Phase 3 Review |
| D-022 | 当前不注册通用 `run_command`、任意 shell/script 或任意 stdin Tool | Accepted |
| D-023 | `ResolvedAction` 必须基于当前 SSH session 的 `LinuxExecutionProfile` 和版本化证据生成，不按泛化的“Linux”猜命令 | Accepted |
| D-024 | resolver 必须生成结构化 `CommandIR`，由 shell renderer 安全渲染；结果由同版本的 tool-specific output parser 解释 | Accepted |
| D-025 | profile/capability 不足或无匹配 resolver 时只能显式探测或返回 unsupported，不得尝试“可能可用”的命令或任意 shell fallback | Accepted |
| D-026 | 参数、执行画像、ResolvedAction、实际提交字节、审批对象和输出使用域隔离 digest 串联；执行前必须核对 revision 与 digest | Accepted |

## 2. 目标、范围与非目标

### 2.1 目标

Terminal Agent 必须在不绕过 JumpServer 现有权限和审计体系的前提下提供：

- 当前终端上下文解释、错误分析和下一步建议；
- 命令补全、ghost suggestion 和命令预览；
- 语义化只读诊断工具；
- 经策略/审批的单步变更；
- 当前 active PTY 内的长命令观察、中断和用户接管；
- 可恢复的 Agent 对话、tool、approval 和 execution 审计。

### 2.2 首版范围

- 一个 Agent session 绑定一个真实 `terminal_session_id`；
- 只支持 SSH/Linux terminal；当前 Tool Registry、resolver、risk 和测试均以该 profile 为边界；
- 先上线解释、建议、确定性补全，再开放固定 resolver 的只读命令；
- 命令型工具只操作当前 active terminal；
- 模型不可直接访问资产凭据或建立 SSH 连接。

### 2.3 非目标

- 关闭浏览器后继续执行的无人值守任务；
- 同时操作多个资产的批处理 Agent；
- 定时巡检和长期后台 Runbook；
- 通过 hosted shell、computer use、remote MCP 直接操作资产；
- 用 LLM 替代 Command ACL、复核、Core 权限或敏感数据 masking；
- 为获得结构化输出而隐式新建第二条 SSH 连接。
- Telnet、K8s container、数据库、Redis/Mongo 和 Windows/PowerShell terminal tools；
- SFTP 上传下载及其他非 terminal 文件协议 Tool；

## 3. 当前系统基础

Koko 当前 active terminal 的核心链路为：

```text
Browser/xterm or shared writable user
  -> Room.Receive(DataEvent)
  -> SwitchSession userInputMessageChan
  -> Parser.ParseUserInput
  -> command ACL / warning / review
  -> userOutChan
  -> srvConn.Write
  -> current active remote PTY

srvConn.Read
  -> Parser.ParseServerOutput / TerminalParser
  -> command record + ReplayRecorder
  -> Room.Broadcast
  -> Browser/xterm
```

可直接复用：

- `SwitchSession.Bridge` 的终端生命周期和字节数据面；
- Parser 的 command ACL、warning/review、ZMODEM 和 terminal mode 检测；
- ReplayRecorder、CommandRecorder 和 session lifecycle；
- Room 的主用户/共享用户汇流；
- Redis Room 的跨 Koko 节点基础路由；
- xterm、现有 Vue Terminal UI 和 session 认证。

必须补齐：

- Agent session 与真实 terminal 的可信 binding；
- 服务端输入所有权、revision、提交回执和幂等；
- Parser readiness snapshot 和 execution lifecycle event；
- tool call、execution、command record、replay 的稳定关联；
- 允许状态未知的异步 ToolResult；
- 统一 ReviewCoordinator 和可恢复 approval。

## 4. 总体架构

```text
Vue Terminal + Agent Drawer
        |
KokoAgentTransport / Completion Client
        |
TerminalAgentService
  +-- Session / Run State Machine
  +-- Context Builder / Redactor
  +-- Model Provider Registry
  +-- Agent Loop / Plan / Tool Router
  +-- Approval Coordinator
  +-- Event Repository / Audit
        |
CommandGuard / ToolPolicy / Core Authorization
        |
ActiveTerminalBindingRegistry
        |
AgentInputGateway + InputLease
        |
Room / SwitchSession input queue
        |
Parser -> ACL / Review -> srvConn -> Recorder
        |
Current active remote PTY
```

横向原则：

- 模型流、tool request、审批、执行和 UI 投影必须分层；
- Go domain/application 不依赖 Gin、WebSocket、OpenAI SDK、`proxy.Server` 或 `srvconn`；
- 浏览器只投影状态和提交用户决定，不掌握凭据、执行事实或策略权威；
- completion 与 Agent Runtime 共享 SessionContext/Metadata/Redactor，但不共享流状态机；
- Agent Runtime 异常不能影响已有 terminal byte stream。

## 5. 技术栈决策

### 5.1 后端

| 领域 | 选择 | 原因与边界 |
| --- | --- | --- |
| 语言 | Go `1.26` 基线 | 与当前 Koko 一致；并发、context、部署和维护统一 |
| 架构 | Hexagonal / Ports and Adapters | SDK、Core、terminal 和 transport 类型不进入 domain |
| OpenAI | `github.com/openai/openai-go/v3`，Responses API | typed stream/tool items；固定经过契约测试的明确版本，当前研究基线 `v3.42.0` |
| OpenAI-compatible | Koko 自有 `openai_compatible_chat` adapter | 不假定兼容服务完整支持 Responses/strict schema/usage |
| 其他模型 | 独立 adapter | Anthropic/Gemini/云厂商/私有模型不得泄漏 SDK 类型 |
| Shell parser | `mvdan.cc/sh/v3/syntax` + 有限 `RecoverErrors` | Go 原生，适合交互式未完成输入；失败时回退轻量 tokenizer |
| LSP | 可选 `CompletionProvider`，Go LSP types/client | 不作为 Shell 首版依赖；不能持有生产凭据或绕开 MetadataProvider |
| 持久化 | `Repository` port + Koko/Core 权威存储 | 禁止用 `sync.Map` 作为会话事实源；事件先持久化再投递 |
| 内部执行接入 | `ActiveTerminalBinding + AgentInputGateway` | 不暴露 SSH client、账号 secret 或任意 `srvConn` writer |

运行时不部署 Node sidecar。`bash-language-server`、`sqls` 等只能作为行为参考或隔离试点，不能直接持有生产 DSN/SSH 私钥。

### 5.2 前端

| 领域 | 选择 | 原因与边界 |
| --- | --- | --- |
| UI | Vue 3 + TypeScript + 现有组件体系 | 保持 Koko 单一前端技术栈 |
| Terminal | xterm | 继续作为真实交互 PTY；Agent UI 不替代 xterm |
| Agent 状态 | `@ai-sdk/vue` `useChat` | 管理 message/tool/approval/status；封装为 `useTerminalAgent()` |
| UI 流协议 | `ai` 的 `UIMessage`/`UIMessageChunk` | Go 生成最小兼容 DTO；domain 不保存 AI SDK 类型 |
| AI completion | `@ai-sdk/vue` `useObject` | 结构化 partial JSON、abort/loading/error/schema validation |
| Schema | Zod + Go canonical DTO contract | 前后端双边校验，未知类型拒绝或安全忽略 |
| 构建基线 | Node 22 + 兼容 TypeScript/vue-tsc | Node 只属于构建链，不进入生产 Agent runtime |
| UI 参考 | AI Elements 的交互模式 | 不引入 React、shadcn/Radix 或 React island |

`ai`/`@ai-sdk/vue` 必须固定经 contract/golden test 验证的精确版本。当前研究基线为 `ai 7.0.20`、`@ai-sdk/vue 4.0.20`，实际升级必须走依赖 Review，不能引用 `latest`。

### 5.3 传输

```text
existing terminal WebSocket                 -> PTY bytes only
/koko/ws/agent                              -> message/plan/tool/approval/events
/koko/api/terminal/completions              -> deterministic short request
/koko/api/terminal/ai-completion            -> useObject-compatible raw JSON stream
```

- Agent 流不得塞进现有 terminal WebSocket handler；
- Agent WebSocket 使用版本化 envelope、request ID、sequence、resume 和单 writer；
- Go 输出 `UIMessageChunk` JSON，但内部持久化 canonical domain event；
- `useObject` endpoint 返回 raw JSON body，不包装 SSE `data:`；
- terminal session binding 必须校验 Cookie 用户、ConnectToken、session owner、资产/账号和 Core 权限，不能相信浏览器传入的 ID。

## 6. Active PTY 执行设计

### 6.1 最终选择

所有会在当前资产执行命令或改变 shell/session 状态的 tool 必须通过当前 active PTY：

```text
typed tool call
  -> deterministic resolver
  -> CommandGuard.Preflight
  -> Approval（如需要）
  -> ActiveTerminalBinding.Resolve
  -> AgentInputGateway.Submit
  -> InputLease / revision / readiness
  -> existing Parser / ACL / Review
  -> current srvConn.Write
  -> active PTY
  -> Parser / Recorder execution events
  -> asynchronous ToolResult
```

选择原因：

- 保留当前 PWD、export、alias、virtualenv、shell option、sudo 和交互状态；
- 复用用户已经授权的账号、跳板链路和网络上下文；
- 复用现有 Parser ACL、Core review、录像和命令记录；
- 用户在当前窗口可见、可中断、可接管；
- 避免第二 SSH 连接造成状态、身份和审计分裂。

代价：PTY 混流，不能天然分离 stdout/stderr；exit code 可能未知；同一 PTY 不能并行；全屏/密码/交互程序必须降级人工。设计必须诚实表达这些限制。

### 6.2 禁止路径

- ToolRouter 直接调用 `srvConn.Write`；
- 只调用 `Room.Receive(command + Enter)`，没有 ack/lease/revision；
- 等到类似 PS1 的文本就返回 exit code 0；
- terminal busy/unknown 时静默另开 SSH exec；
- Agent 自动发送 `y` 绕过 Parser review；
- 模型回答密码、MFA 或 host-key 确认；
- 取消模型 HTTP 请求时自动假定远端命令已终止。

### 6.3 Binding 与 Submit

`ActiveTerminalBinding` 必须包含：

- terminal/asset/account/protocol 和 owner Koko node；
- session/capability/input revision；
- readiness、control owner 和 lease version；
- 不包含账号 secret、SSH client 或裸 writer。

`PTYExecutionRequest` 必须绑定：

- agent session、run、tool call；
- immutable `ResolvedAction`/`resolved_action_digest`、exact submission payload/`command_bytes_digest`；
- terminal session、expected input/capability revision；
- approval ID、`approval_subject_digest`、policy revision、expiry；
- idempotency key。

Submit ack 只表示 owner node 已持久化并接受 execution，不表示执行成功。

### 6.4 InputLease

- 同一 terminal 最多一个 Agent execution 持有 lease；
- 命令 tool 按 `tool_call.created_seq` 严格串行；
- lease 绑定 execution ID、version、input revision；迟到 version 一律拒绝；
- 自动提交前逻辑输入必须为空；有用户未提交输入时返回 `input_conflict`；
- Agent 持有 lease 时普通人类输入不得无提示混入命令 stdin；
- 用户 Take over、管理员终止和紧急 interrupt 高优先级撤销 lease；
- 用户接管后所有 Agent 写入必须在服务端失败；
- reject、review cancel、terminal outcome、断线、超时和进程恢复都必须释放或 reconcile lease。

### 6.5 Readiness 门禁

以下条件全部满足才可以自动提交：

- session 在线、权限有效、未 pause；
- Parser 已初始化且 revision 匹配；
- 位于可信 prompt/input 状态，输入 buffer 为空；
- 无前台命令和其他 Agent lease；
- 不在 password/MFA/host-key prompt；
- 不在 alternate screen/editor/TUI；
- 不在 ZMODEM；
- 不在 warning/review query 或 review running；
- profile 明确支持 active-PTY command。

首版在 `tmux/screen` 中降级 propose-only。门禁失败返回稳定 reason，绝不自动 fallback。

### 6.6 Execution 状态

```text
proposed
  -> awaiting_approval
  -> queued
  -> submitted_to_parser
  -> parser_review
  -> written_to_pty
  -> running
  -> completed | interrupted | rejected | disconnected | unknown | failed
```

每个事件必须包含 `execution_id + sequence + timestamp`。Agent WebSocket 重连只恢复事件，不重新提交命令。

### 6.7 输出与完成语义

必须区分：

- `accepted`：Gateway 接受请求；
- `written`：Parser 允许且已写入当前 srvConn；
- `observed`：捕获到有限 terminal output；
- `completed`：可信 hook/协议确认完成；
- `exit_code`：只有可信信号提供时才设置。

ToolResult 使用 `terminal_output/output_snapshot`，不伪称 stdout/stderr。完成信号优先级：可信 shell integration → 协议信号 → 稳定 prompt 仅标记 likely completed → 无信号保持 running/unknown。

不得修改命令拼接可伪造 sentinel。等待超时只停止等待，不重跑或宣称命令失败。

### 6.8 长命令与接管

- 短观察窗口未完成则返回 `running + execution_id + bounded snapshot`；
- `inspect_execution` 只读现有状态，绝不重跑；
- `InterruptExecution` 只有在 execution 仍绑定当前前台命令时才能发送受审计 Ctrl+C；
- 用户接管只改变 control owner，不等于命令完成；
- password、MFA、交互选择和全屏应用进入 `needs_user_takeover`；
- 无法确认中断结果时状态为 `interrupted/unknown`。

### 6.9 Review 协调

Tool preflight 和 Parser 实时 authorize 都必须保留，但不能要求用户审批两次：

- approval 通过 `approval_subject_digest` 绑定 resolved action、tool call/run、terminal session、policy/capability revision、scope 和 expiry；
- Parser review 通过结构化 event 接入统一 ReviewCoordinator；
- continuation 必须鉴权、幂等并再次核对 digest/revision；
- Agent 不能用 terminal `y` 模拟批准；
- 人类 terminal `y/n` 与 ApprovalCard 必须归并到同一个 review state。

### 6.10 跨节点与副作用边界

- Binding Registry 必须定位 session owner node；
- Submit 需要 request/ack 和 durable idempotency，不能只依赖 Redis pub/sub；
- 重复 idempotency key 返回已有 execution；
- “可能已写 PTY但结果未知”时进入 `NEEDS_RECONCILIATION`，不得重放；
- Agent Runtime 关闭不能关闭用户的 srvConn；
- owner node/session 断开将 execution 终结为 `disconnected/unknown`。

内部 RPC 还是可靠 Redis 机制由 D-017 决定，首版实现前必须完成故障注入和压测。

## 7. Agent Runtime

### 7.1 分层

```text
Transport
  -> Application Service
     -> AgentLoop
        +-- ContextBuilder
        +-- ModelClient port
        +-- ToolRegistry/Router
        +-- Policy/Approval
        +-- ActivePTYExecutor port
        +-- Repository/EventSink
```

一个 Agent session 同时最多一个 active run。一个 step 可以并行调用纯控制面只读工具，但同一 active terminal 的命令 tool 固定 `SupportsParallel=false`。

### 7.2 Tool loop

1. 构造脱敏、限长、有来源的上下文；
2. 调用 ModelClient，持久化 typed stream event；
3. 收到 tool call 后严格 schema decode；
4. 解析 semantic tool，生成 deterministic action；
5. 计算 capability、risk、policy 和 approval；
6. claim idempotency key；
7. 控制面工具直接执行，命令工具提交 ActivePTYExecutor；
8. 持久化 ToolResult，再回传下一轮模型；
9. 达到 finish/预算/取消/失败条件后写明确 terminal outcome。

### 7.3 重试边界

- 尚未收到任何模型输出/tool action 时，可以有限重试相同 provider 请求；
- 收到任何可执行 action 后禁止透明重放原请求；
- action 已排队/已写 PTY后，只能从 execution checkpoint 继续；
- provider 首个 event 后禁止静默切换厂商；
- WebSocket/UI 序列化失败不能导致 tool 重跑。

### 7.4 取消

UI Stop、`/stop`、权限失效和管理员终止统一进入 `CancelRun`：取消模型、拒绝 pending approval、停止未提交 action、更新 run 状态并审计。已经写入 PTY 的命令只有显式 `InterruptExecution` 才会尝试 Ctrl+C。

## 8. Tool、能力与风险

### 8.1 当前范围

本设计只定义附着于当前 SSH/Linux terminal 的 Tool。以下类别不进入当前 Tool Registry：

- Telnet、K8s、数据库、Redis/Mongo、Windows/PowerShell profile；
- SFTP 上传下载、端口转发等非 terminal 协议动作；
- 多资产、后台、定时 Task Agent tools；
- hosted shell、computer use、remote MCP；
- 模型动态注册的 tool。

未来扩展其他协议时必须另建对应设计并新增决策，不能只给现有 SSH tool 换一个命令模板。

### 8.2 Tool 是否固定

Tool 由后端固定注册、版本化并通过 CapabilitySet 下发。模型只能选择可见 Tool 并填写参数，不能创建 Tool、修改 Schema、风险或 resolver。

| 层次 | 稳定性 | 规则 |
| --- | --- | --- |
| Tool 名称/语义 | 固定 | 由后端 `ToolRegistry` 注册，有稳定 `tool_version` |
| 参数 Schema | 固定、版本化 | strict decode；未知字段、越界值和非法枚举拒绝 |
| 当前可见集合 | 动态收缩 | 根据用户、SSH session、资产账号、ACL、readiness 和 rollout 过滤 |
| 实际命令 | 后端确定性生成 | resolver 按 Linux distro/init system/shell 选择受信模板 |
| 风险/审批 | 服务端计算 | 模型标签不影响最终策略 |
| 输出 | 动态但有界 | 每 Tool 固定提取策略、timeout、max lines/bytes 和敏感字段规则 |

每个 ToolDefinition 必须包含：

- `name/version/description/input_schema/output_schema`；
- `resolver_version` 和支持的 Linux/profile 条件；
- 默认 risk、是否可能改变 shell/资产状态；
- readiness 前置条件、timeout、最大输出；
- approval/ACL policy hook；
- command preview、`resolved_action_digest` 和 `command_bytes_digest`；
- redaction、audit 和验证规则；
- 是否允许模型调用，还是仅供用户/Runtime 内部控制。

管理员发布的 Runbook 可以组合既有 Tool 或增加经过审核的版本化 ToolDefinition，但模型不能在一次 run 中临时发明工具。

#### 8.2.1 为什么不能只按“Linux”生成命令

`ResolvedAction` 不是模型写出的一段 shell 文本，而是 Koko 根据当前会话事实解析出的、可授权和可执行的确定性动作。同一个 `inspect_service` 在 systemd、OpenRC、SysV/BusyBox 上的命令、输出和成功语义不同；`inspect_port` 可能使用 `ss`、`netstat` 或 `lsof`；包查询也分别属于 dpkg、rpm/apk/zypper 体系。即使资产 OS 相同，当前会话还可能处于 restricted shell、容器/chroot、`sudo -i`、`su`、嵌套 SSH 或不同 `PATH` 中。

因此，仅凭资产标签、模型常识或历史上一次连接选择命令会产生三类问题：

- **正确性错误**：命令不存在、flag 语义不同、输出 parser 误判；
- **安全错误**：审批时看到的动作与实际 fallback 命令不同，或者在错误的嵌套会话执行；
- **恢复错误**：缓存命令跨会话复用，重连后把旧环境的 action 写入新 PTY。

本设计选择“会话事实 → 确定性 resolver → 冻结动作 → 审批 → 同一 active PTY”的原因，是把模型的不确定推理终止在 ToolCall 层；越靠近资产执行，语义必须越确定。

#### 8.2.2 会话级 `LinuxExecutionProfile`

每个 active SSH terminal 维护独立、单调递增 revision 的执行画像。它至少包含：

```text
LinuxExecutionProfile
  identity: org/user/asset/account/terminal_session opaque IDs
  revision, collected_at, expires_at
  distro: id/version/variant (known | unknown)
  arch, kernel
  shell: family/version/restricted (known | unknown)
  init_system: systemd/openrc/sysv/busybox/other/unknown
  package_system: dpkg/rpm/apk/zypper/other/unknown
  commands: name -> present/version/feature evidence
  filesystem: procfs presence, known cwd
  context: host/container/chroot/nested_ssh/unknown
  locale and PATH evidence
  evidence[]: source, observed_at, command_execution_id/ref, confidence class
```

画像的作用仅是**兼容性路由**，不是权限或可信身份来源。远端机器完全可能返回伪造的 `os-release` 或命令输出，因此授权仍以 Core、ConnectToken、当前 binding 和 CommandGuard 为准。

画像证据按下列顺序合并，并保留来源：

1. Core/SSH connection 已知事实：资产、账号、协议和连接时的 shell hint；
2. 经过协议化的 shell integration 事件：shell、PWD、命令边界和上下文变化；
3. 当前 session 已观察到的 command record、`command not found` 和结构化结果；
4. 用户可见且受审计的 `inspect_system(sections=["os","init","commands"])` 探测。

Core 中的静态 OS 标签不能覆盖当前 PTY 的相反证据。探测本身也是当前 active PTY 中的一次固定 Tool execution，必须经过 readiness、InputLease、Parser/ACL、录像和输出限制，不能作为隐藏 completion generator 偷跑。连最小探测命令都无法安全选择时，返回 `profile_unknown/unsupported`，交还用户处理。

画像缓存 key 至少包含 `org + user + asset + account + terminal_session + shell context`，不得只按 asset 缓存。出现以下事件时必须提升 revision、使未执行 `ResolvedAction` 失效，并按需重新探测：

- 重连、session/binding/账号变化；
- `exec`、`su`、`sudo -s/-i`、容器/chroot、嵌套 SSH 或 shell 切换；
- PWD、`PATH`、restricted-shell 状态或关键 command availability 变化；
- resolver 收到 `command not found`、不支持的 flag/输出格式；
- profile TTL 到期或 shell integration 报告 desync。

无法可靠判断上下文是否已变化时，状态必须降级为 `unknown`，不能沿用旧画像。该保守选择会降低自动化覆盖率，但能避免把命令发到用户没有审批的运行环境。

#### 8.2.3 版本化 `ResolverRegistry`

resolver 以 `tool name/version + profile predicate + resolver version` 注册，例如：

```text
inspect-service/systemd-v2
inspect-service/openrc-v1
inspect-service/sysv-v1
inspect-port/iproute2-ss-v2
inspect-port/net-tools-v1
```

选择规则必须确定、可测试且无模型参与：

1. strict decode 并语义化校验 Tool 参数；
2. 加载仍在有效期内的最新 profile/capability/policy revision；
3. 只选择 predicate 完全满足的最高优先级已发布 resolver；
4. 多个 resolver 同优先级匹配视为 registry 配置错误并 fail closed；
5. 没有匹配时请求显式 profile 探测，仍未知则返回 `unsupported_profile`。

不能生成 `systemctl ... || service ...`、`ss ... || netstat ...` 一类在资产上自行猜测的 fallback。它会让实际分支无法在审批前确定，也让 output parser 无法知道输出来自哪套语义。若一次只读 action 暴露了新证据，Runtime 可以更新 profile 后**重新解析为新的 action**；新 action 使用新的 digest，并按风险策略重新授权和审计。

#### 8.2.4 `CommandIR`、renderer 与输出 parser

resolver 先生成结构化 `CommandIR`，不得使用 `fmt.Sprintf` 把用户字段拼入 shell：

```text
CommandIR
  program: resolver 常量或受信 executable ID
  args[]: typed literal arguments
  env[]: allowlisted key/value（例如 resolver 明确需要的 LC_ALL=C）
  cwd_precondition: optional expected PWD
  stdin: none（当前 SSH tools）
  submit_delimiter: CR/LF policy
  timeout/max_output
  output_parser_id/version
```

首版 `CommandIR` 不允许自由 shell fragment、command substitution、redirection、heredoc 或任意 pipeline。确需组合时必须扩展为有类型的 stage/redirect 节点并单独评审；不能退化成字符串字段。shell-specific renderer 只负责把 IR 安全转为当前 shell 的精确 UTF-8 字节，并返回不可再修改的 submission payload。

每个 resolver 必须绑定同版本语义的 output parser。优先选择稳定、字段明确的输出，例如 systemd 使用受限 `systemctl show --property=...`，`ps` 指定固定列，`df` 使用经过该平台验证的格式；不优先解析面向人的彩色 `status` 页面。`journalctl -o json`、`ss -H` 等 flag 只有在 profile 有功能证据时才能使用。parser 遇到 locale、字段、截断或退出状态不确定时返回 `partial/unknown` 和原始受限证据，不能推断“服务已停止”或伪造成功。

不同发行版不是共享一个不断追加条件分支的大模板，而是共享 Tool 语义、分别实现小型 resolver/parser。这样升级某个平台不会静默改变其他平台的审批对象，resolver version 也可以精确回滚。

#### 8.2.5 `ResolvedAction` 必须携带的来源

```text
ResolvedAction
  action_version
  tool_name/tool_version/tool_call_id
  normalized_arguments + arguments_digest
  resolver_id/resolver_version/output_parser_version
  linux_profile_revision + linux_profile_digest
  capability_revision
  terminal_binding: org/user/asset/account/session opaque IDs
  command_ir
  display_preview
  submission_length + command_bytes_digest
  timeout/max_output/redaction policy
  computed_risk
  resolved_at/expires_at
  resolved_action_digest
```

`display_preview` 是给人阅读的表现层，不能作为执行或 digest 输入的唯一来源；转义、换行折叠、Unicode 显示宽度都可能让两个不同字节串看起来相同。真正提交的 bytes 在 resolver 完成后冻结，CommandGuard、审批卡片、InputGateway 和审计都引用同一个 `ResolvedAction`，任何一层都不能重新从参数生成命令。

执行前必须重新读取 terminal binding、profile、capability、policy、InputLease 和 input revision。若影响动作语义的 revision、PWD 前置条件或 payload digest 发生变化，旧 action 进入 `STALE_RESOLUTION`，重新 resolve；需要审批的动作必须以新 digest 重新审批。禁止“只刷新命令但沿用旧 approval”。

#### 8.2.6 Digest chain 的定义

首版统一使用 SHA-256，编码为 `sha256:<lowercase-hex>`。结构化对象使用 RFC 8785 JSON Canonicalization Scheme（JCS）编码；字符串保留原始 Unicode code point，不做会改变 Linux 文件名语义的隐式 NFC/NFD 转换。所有 digest 都必须带域隔离前缀和版本：

```text
digest = SHA-256("koko:ssh-agent:<object-kind>:v1\0" || canonical_bytes)
```

至少定义以下对象，且不得混用：

| Digest | 覆盖内容 | 意义 |
| --- | --- | --- |
| `arguments_digest` | strict decode/语义校验后的参数 | 识别参数在模型输出、resolver 和审计之间是否变化 |
| `linux_profile_digest` | 影响 resolver 的 profile facts、revision 和证据引用 | 证明动作是基于哪一版会话环境解析，不代表远端事实可信 |
| `command_bytes_digest` | InputGateway 将提交的**完整精确字节**，包括提交分隔符 | 防止 preview 相同但实际字节不同，也防止审批后修改 quoting/flag/目标 |
| `resolved_action_digest` | 除自身 digest/display-only 字段外的规范化 ResolvedAction，包含上述 digest、binding、resolver 和约束 | 给 action 一个稳定内容身份，串联策略、审批、执行和恢复 |
| `approval_subject_digest` | action digest、tool call/run/session、policy/capability revision、审批 scope 和 expiry | 限定“谁批准了哪个会话的哪次动作，在什么策略和期限下有效” |
| `output_digest` | execution correlation 后保存的原始受限输出 bytes 或受控 `ToolOutputRef` 内容 | 检测结果在保存、裁剪、模型消费和审计之间是否变化 |

域隔离的原因是：即使两个对象偶然有相同 canonical bytes，也不能把 `output_digest` 当成 `approval_subject_digest` 使用。版本字段使未来更换 canonicalization、hash 或字段集合时能够并行验证旧记录，而不是静默改变历史语义。

`command_bytes_digest` 必须在 InputGateway 写入第一字节前重新计算并与 frozen action 比较。若底层 write 只接受了部分字节，write receipt 记录 `accepted_length`、已接受前缀的 digest 和 `partial/unknown`，禁止自动重发。完整成功时 `written_bytes_digest` 必须等于 `command_bytes_digest`；golden test 验证的也是这份 exact payload，而不是日志中的 command string。

`output_digest` 应区分受控原始 evidence 与发送给模型的 redacted/truncated view；二者分别计算 digest，并由 result manifest 关联。否则无法判断内容差异来自篡改，还是正常的裁剪与脱敏。

#### 8.2.7 Digest 的具体意义与边界

digest 在本设计中解决的是**内容完整性和跨阶段关联**：

- 把“模型请求 → resolver 结果 → 用户审批 → PTY 实际 bytes → ToolResult”连成可核对的证据链；
- 在 profile、policy、capability 或参数变化时可靠识别 TOCTOU，并触发重新解析/审批；
- 为幂等提交、崩溃恢复、跨节点 continuation 和重复 action/result 去重提供稳定 key；
- 让审计能够回答“审批看到的是否就是最终写入的内容”，而不是依赖容易变化的 UI 文本；
- 让 resolver golden fixture 和生产 execution receipt 使用同一个可比较事实。

digest **不等于**授权、签名、加密或执行证明：

- 它不证明远端真的执行成功，也不证明远端输出真实；PTY 结果仍可能是 `unknown`；
- 它不替代 Core 权限、CommandGuard、InputLease、审批身份和 expiry；
- SHA-256 不隐藏低熵参数或 secret，敏感数据仍必须脱敏/加密，不能因为只保存 digest 就认为安全；
- 普通 digest 不能抵抗有权同时修改数据和重算 hash 的数据库攻击者。若审计威胁模型包含存储篡改，必须另加服务端密钥 HMAC/签名、append-only event chain 或外部不可变审计存储。

明确这些边界的意义是避免把 hash 当成“安全万能证明”。本设计使用 digest 锁定对象内容，使用鉴权/审批决定是否允许，使用 active PTY receipt 说明是否提交，使用 Parser/record/result 说明观察到什么；四者职责不可互相替代。

#### 8.2.8 端到端解析流程

```text
ToolCall
  -> strict schema + semantic validation
  -> load fresh LinuxExecutionProfile / capability / binding
  -> select exact versioned resolver
  -> CommandIR
  -> shell-specific safe renderer
  -> freeze ResolvedAction + digest chain
  -> CommandGuard / policy / preview / approval
  -> recheck revisions + approval subject + command bytes digest
  -> ActiveTerminalBinding / AgentInputGateway / InputLease
  -> existing Parser / ACL / Review -> current active PTY
  -> resolver-specific output parser
  -> bounded raw evidence + redacted model view + separate digests
  -> update profile evidence when applicable
```

该流程仍然能够支持 ReAct：模型负责基于 observation 选择下一个语义 Tool 和参数，Koko 负责把 action 确定化并执行。固定 resolver 不会削弱 ReAct 的“Reason + Act”循环，只是禁止模型把 Act 偷换成不受控 shell；当返回 `unsupported_profile` 或 `unknown` 时，模型可以选择 `inspect_system`、换用已注册的其他 Tool、解释限制或 `handoff_to_user`。

### 8.3 SSH Terminal 上下文与提议 Tool

这些 Tool 不向远端发送新命令：

| Tool | 参数摘要 | 数据来源 | 风险/说明 |
| --- | --- | --- | --- |
| `inspect_terminal` | `max_lines` | Parser/VT snapshot | R0；读取当前屏幕、prompt/readiness、PWD（已知时） |
| `inspect_ssh_session` | 无 | Binding/Core | R0；脱敏资产、账号、OS/shell、session expiry、control owner |
| `query_command_history` | `prefix/limit` | Koko/Core command records | R0；必须按用户/资产/账号隔离 |
| `propose_command` | `intent` | Agent/UI | R0；只生成 preview/fill，不发送 Enter |
| `explain_command` | `command` | 本地 parser/model | R0；不执行 |
| `explain_terminal_error` | `execution_id?` | bounded terminal context | R0；输出视为 untrusted |

`inspect_asset` 不单独作为通用资产 Tool；SSH Agent 只通过 `inspect_ssh_session` 获得完成当前诊断所需的最小脱敏信息。

### 8.4 SSH Execution 控制 Tool

这些 Tool 控制已经存在的 active-PTY execution，不生成新的业务命令：

| Tool | 参数摘要 | 调用者 | 规则 |
| --- | --- | --- | --- |
| `inspect_execution` | `execution_id/max_lines` | 模型或用户 | 只读现有状态，不重跑命令 |
| `wait_execution` | `execution_id/wait_ms` | Runtime 内部或模型 | 有最大等待；超时仍保持 running/unknown |
| `interrupt_execution` | `execution_id` | 用户批准或策略允许 | 仅当 execution 仍绑定当前前台命令时发送受审计 Ctrl+C |
| `handoff_to_user` | `execution_id/reason` | 模型或 Runtime | 原子撤销 Agent lease，用户接管 |

`resume_after_handoff` 是用户操作/API，不是模型 Tool。重新申请 lease 必须使用新的 version 并重新验证 readiness。

### 8.5 首批 SSH 只读命令 Tool

这些 Tool 由后端 resolver 生成固定、可预览的命令，并通过当前 active PTY 执行：

| Tool | 主要参数 | 典型 resolver 范围 | 风险与限制 |
| --- | --- | --- | --- |
| `inspect_system` | `sections[]` | `hostname`、`uname`、`uptime`、受限 `/etc/os-release` | R0；sections 枚举，不接受命令片段 |
| `inspect_resource_usage` | `resource/limit` | 内存、load、CPU 摘要 | R1；固定采样次数和输出上限 |
| `inspect_disk` | `path/include_inode` | `df`、受限目录统计 | R1；path 规范化，首版禁止递归全盘扫描 |
| `inspect_process` | `pid/name/sort/limit` | `ps`/固定过滤 | R1；禁止将参数拼成自由表达式 |
| `inspect_port` | `port/protocol/state` | `ss` 或平台受信替代 | R1；port 范围、protocol/state 枚举 |
| `inspect_service` | `service/lines` | systemd/service status | R1；service name 严格校验，不接受额外 flags |
| `read_log` | `source/since/lines/match` | `journalctl` 或允许目录中的日志读取 | R1/R2；source allowlist、时间/行数/字节上限，敏感日志可要求审批 |
| `test_network` | `mode/target/port/count` | DNS、路由、有限 TCP/ping 检查 | R1；目标/端口校验，禁止扫描网段和无限次数 |
| `inspect_package` | `name` | rpm/dpkg/package-manager query | R1；只查询，不刷新仓库、不安装 |
| `inspect_file` | `path/start_line/max_lines` | 受限文本读取 | R2；路径策略、大小、类型和 secret scan；默认不开放敏感目录 |

首版推荐先开放 `inspect_system`、`inspect_disk`、`inspect_process`、`inspect_port`、`inspect_service` 和有限 `read_log`，再根据 unknown rate、输出脱敏和 ACL 数据开放其他 Tool。

#### 固定 resolver 示例

模型只能请求：

```json
{
  "tool": "inspect_service",
  "arguments": {
    "service": "nginx",
    "lines": 80
  }
}
```

后端完成：参数严格校验 → 检测 init system → resolver 生成规范化命令 → risk/ACL/approval → 显示 preview → active PTY。模型不能追加 flags、管道、redirection 或 `;` 后续命令。

### 8.6 SSH 会话状态变更 Tool

这些操作不一定修改资产，但会改变当前 shell 上下文，必须通过 active PTY 并对用户可见：

| Tool | 参数 | 风险/阶段 |
| --- | --- | --- |
| `change_directory` | `path` | R1；路径规范化，当前输入必须为空；Phase 3 后评估 |
| `activate_environment` | `environment_id` | R2；只能选择后端发现/管理员配置的环境，不能传任意 source 脚本 |

首版不提供通用 `set_environment`、`export_secret`、shell option 修改或任意启动脚本。

### 8.7 后续 SSH 变更 Tool

只有只读 Tool、approval、rollback 和 active-PTY correlation 达到上线门禁后才评估：

| Tool | 参数必须结构化 | 风险/要求 |
| --- | --- | --- |
| `manage_service` | `service/action=start|stop|restart|reload` | R2/R3；精确审批和 post-check |
| `manage_process` | `pid/signal` | R2/R3；signal allowlist，KILL 单独高风险 |
| `apply_config_patch` | `path/base_hash/patch` | R3；展示 diff、备份、语法 pre-check、rollback |
| `manage_file` | `action/path/destination/mode` | R2/R3；路径/符号链接/权限边界 |
| `manage_package` | `name/action/version` | R3；仓库和包 allowlist，禁止模型注入参数 |
| `manage_permission` | `path/owner/group/mode` | R3；不得扩大到未授权主体 |
| `manage_firewall_rule` | 结构化 protocol/source/port/action | R3；Core 工单、连接自保护和 rollback |
| `execute_runbook_step` | `runbook_version/step_id/inputs` | 由固定步骤决定；只能执行已发布 Runbook |
| `rollback_change` | `original_execution_id` | 只执行原审批绑定的预定义 rollback |

用户、SSH key、sudoers 等身份权限管理不进入当前阶段；如未来需要，必须单独安全评审，而不是复用 `manage_file` 绕过。

### 8.8 不提供的 SSH Tool

当前明确不注册：

- `run_command(command: string)`；
- `execute_shell(script: string)`、`eval`、任意 heredoc；
- `sudo_command` 或模型输入 sudo 密码；
- 任意 Python/Perl/Ruby/Node 解释器脚本；
- 任意 stdin 连续写入；
- `nohup`/后台脱离 terminal 的任务；
- 任意文件删除、用户管理、SSH key/sudoers 修改；
- 模型创建/修改 ToolDefinition 或 Runbook；
- 将多个隐藏命令封装成不可审计的 `diagnose_everything`。

复杂诊断由 Agent 编排小而稳定的 Tool，例如：

```text
inspect_service
  -> inspect_port
  -> inspect_process
  -> read_log
  -> test_network
  -> propose_command
```

每一步都能独立做 capability、ACL、审批、输出限制、取消和审计。

### 8.9 CapabilitySet

CapabilitySet 由服务端根据以下事实动态**减少**固定 SSH Tool 集合：

- org/user 权限和 ConnectToken actions；
- SSH asset/account/session binding 与 expiry；
- Linux distro、init system、可用命令和 shell；
- Parser readiness、active command、control owner 和 lease；
- Command ACL/Core policy revision；
- feature rollout 和 Tool/resolver version。

模型看不到不支持或当前不允许的 Tool。执行前仍重新授权；浏览器传入 capability 不是权威。CapabilitySet 不允许模型或前端动态增加 Tool。

### 8.10 风险等级

| 等级 | SSH Tool 示例 | 默认行为 |
| --- | --- | --- |
| R0 | terminal/session 观察、`inspect_system` 基础字段 | readiness/策略通过后可自动提交或本地读取 |
| R1 | 磁盘、资源、进程、端口、服务查询 | 可自动提交，完整审计和输出上限 |
| R2 | 敏感日志/文件、change directory、service reload | 用户精确审批 |
| R3 | 配置、包、进程终止、权限、防火墙变更 | Core 工单/复核、验证与 rollback |
| R4 | 格式化磁盘、绕过审计、凭据/SSH key/sudoers 攻击 | 永久拒绝 |

模型的 `read_only/risky` 只能作为不可信 hint。CommandGuard 必须处理复合命令、嵌套命令、redirection、编码/转义和参数变更；resolver 生成固定命令也不能跳过 Parser 的实时 ACL/复核。

## 9. 命令补全

### 9.1 产品分层

```text
Completion menu  -> 多候选、确定性、低延迟
Ghost suggestion -> 单候选、只填入、不发送 Enter
Agent conversation -> 独立 tool loop，不参与按键级候选排序
```

### 9.2 Provider 瀑布

```text
context history
  -> recent history（同 PWD 优先）
  -> static/spec
  -> Koko metadata
  -> optional LSP
  -> AI last fallback
```

- history cache 必须按 org/user/asset/account/protocol/PWD 隔离；
- static/history/metadata 在模型不可用时仍工作；
- AI suggestion 必须标来源，不能自动执行；
- 所有候选在实际执行时重新经过 CommandGuard/Core；
- SSH 命令目录、service name 和可选远程路径 metadata 优先来自 Koko/Core 已有事实或独立受控 provider，不向 active PTY 注入隐藏探测命令。

### 9.3 InputTracker 与坐标

Koko/xterm 没有 Warp 自绘 Editor 的完整 buffer model，必须维护：

- `input_revision`、逻辑 buffer、cursor 和 selection；
- UTF-8 byte offset、UTF-16 position、xterm cell 三种坐标；
- request ID + revision + selection snapshot；
- stale result 丢弃和每 channel 独立 cancel；
- alternate screen、未知 CSI、Ctrl+R、Tab、鼠标等导致 desync 时关闭增强补全，等待服务端 snapshot 对齐。

普通 Tab 保留给远端 shell；增强补全/ghost 使用 `Ctrl+Space`、`Alt+Right` 等独立快捷键。

### 9.4 AI completion

- AI completion 使用独立短请求和 `useObject`；
- 只发送脱敏、限长 SessionContext；
- 结果必须符合 versioned schema；
- revision/profile/session 变化时显式 stop；
- 接受只修改逻辑输入，不发送 Enter；
- zero-state next command 风险更高，首版只展示。

## 10. 上下文与模型接入

### 10.1 SessionContext

至少包含：

- terminal/session/asset/account/protocol 稳定标识；
- OS/shell/profile、PWD、可选 namespace/database；
- 最近命令、exit status（已知时）、有限输出摘要；
- Parser readiness、alternate screen、control owner；
- 当前 capability/policy revision；
- 每个事实的来源和时间。

终端输出始终标记为 untrusted data。密码、token、私钥、Cookie、连接串、命令参数、日志敏感字段和组织敏感字段在出 Koko 前脱敏。

### 10.2 Provider port

Provider contract 必须统一：

- typed text/reasoning/tool-call/usage/finish/error events；
- canonical finish reason 和 provider raw metadata；
- capability/warning；
- request/attempt ID；
- cancel 和超时。

OpenAI Responses 每次显式 `store:false`。Koko 保存 canonical conversation/tool/audit；provider opaque state 加密、限长、限期且不进入 UI。

## 11. 前端设计

### 11.1 页面结构

- xterm 保持主执行面；
- Agent Drawer 展示 Conversation、Plan、Tool、Approval、Execution；
- CompletionMenu/GhostSuggestion 作为 overlay，不写入 xterm output buffer；
- active PTY control owner、running execution 和 Take over 必须持续可见；
- risky action 显示规范化命令、资产、账号、风险、审批对象和验证计划。

### 11.2 状态权威

- `useChat.messages` 是前端 message/tool part 投影；
- Koko Repository/domain event 是服务端事实源；
- pending approval、execution 和 run 可在 Drawer 卸载/重连后恢复；
- Pinia 不复制一套独立 conversation 事实；
- UI 不能仅通过隐藏输入或禁用按钮实现 lease/权限。

## 12. 持久化、审计与恢复

必须持久化：

- Agent session/run/step 状态与 revision；
- canonical message/event 和 provider attempt；
- tool schema version、规范化参数及 `arguments_digest`；
- `LinuxExecutionProfile` revision/digest/evidence refs、resolver/parser version 和完整 `ResolvedAction`；
- capability/policy revision；
- approval subject、决定、expiry 和 `approval_subject_digest`；
- execution/idempotency/lease/sequence、预期 `command_bytes_digest` 和 InputGateway write receipt；
- Parser command event、command record ID、replay 时间点；
- bounded ToolResult、原始 evidence/model view 各自的 `output_digest`、truncation handle 和读取审计；
- cancel、takeover、interrupt、disconnect 和 reconciliation 原因。

副作用恢复原则：

- action 前可重试；
- action accepted 后从 checkpoint 继续；
- 可能已写 PTY但结果未知时不得重放；
- 权限、policy、session readiness 在恢复时重新验证；
- 无法恢复时进入 `NEEDS_RECONCILIATION`，不能静默丢弃。

审计必须回答：谁提出目标、模型看到什么、使用哪个 provider/model/prompt、生成什么 tool、哪条策略决定、谁批准、实际写入什么命令、何时进入哪个 PTY、用户是否接管、结果是否确定、输出是否裁剪/脱敏、是否发生重试。

## 13. 安全边界

- Agent/模型不接触资产密码、私钥和 ConnectToken 明文；
- 模型不能建立 SSH、使用 hosted shell/computer use 操作资产；
- Agent 权限不超过发起用户和当前 session；
- ToolRegistry/CapabilitySet/CommandGuard/Core 是权限权威；
- prompt injection 不能修改工具集合、审批或风险策略；
- approval 必须绑定 `approval_subject_digest`；其中包含 `resolved_action_digest`、policy/capability revision、session、scope 和 expiry；
- display preview 不作为执行事实；InputGateway 只接受被冻结且 digest/revision 全部匹配的 submission bytes；
- profile 未知、resolver 不匹配、action stale 或 write partial 时 fail closed，不能换命令或自动重发；
- 模型输出、terminal output、tool output 都是不可信数据；
- typed tool 按字段 sensitivity 脱敏，PTY output 再走 scanner/DLP；
- 同 terminal 命令串行，控制面只读工具按明确策略并行；
- 文件传输、端口转发、sudo、多资产写默认关闭；
- provider、UI 或 WebSocket 故障不得导致重复执行；
- 日志禁止记录 secret、完整敏感 prompt 和未脱敏输出。

## 14. 代码改造边界

### 14.1 现有代码

| 区域 | 改造重点 |
| --- | --- |
| `pkg/proxy/switch.go` | 注册 active binding；装配 owner-node AgentInputGateway/InputLease；关闭时终结 execution |
| `pkg/proxy/parser.go` | Snapshot/readiness/input revision；execution lifecycle；review event/continuation；来源关联 |
| `pkg/proxy/parsercmd.go` | 线程安全输入/光标/prompt/state snapshot；提高 command boundary 可观察性 |
| `pkg/proxy/command_check.go` | 抽取 ReviewCoordinator；approval digest/幂等/恢复 |
| `pkg/exchange/message.go` | 服务端可信 source/run/tool/execution/lease metadata；禁止信任浏览器自报 |
| `pkg/proxy/recorder.go` | execution ↔ command record ↔ replay correlation 和落库 event |
| `pkg/httpd` | 独立 Agent WS、completion endpoints、binding lifecycle |
| `pkg/srvconn/conn_openai.go` | 仅旧 Chat 过渡；不作为新 Agent provider/runtime |
| `ui/src` | Agent transport/composables/components；InputTracker；ghost/menu overlay |

### 14.2 新模块

```text
internal/agent/
  domain/                 session/run/tool/approval/execution/events
  application/            prompt/approve/cancel/resume/agent loop
  ports/                  model/policy/repository/active-terminal/metadata/audit
  adapters/
    llm/                  openai-responses/openai-compatible/contract tests
    active_terminal/      binding/input gateway/lease/parser events/store
    core/                 repository/audit/authorization
    metadata/             SSH command/service/path metadata（可选）
  completion/             providers/parser/ranker/profiles/LSP adapter
  tools/                  registry/resolver/queue/output store
  transport/ws/           envelope/handler/mapper
```

## 15. 分阶段实施

### Phase 0：契约与安全基线

- 定义 session/run/tool/execution/approval/event DTO 和状态机；
- 定义 capability、risk、InputSource、error code；
- 定义 version/sequence/reconnect/idempotency/side-effect boundary；
- 为现有 Parser ACL/review/command record 建 characterization tests。

### Phase 1：只读副驾驶和确定性补全

- 建立可信 terminal snapshot 和 InputTracker；
- 建立 session-scoped `LinuxExecutionProfile`、evidence/revision/TTL/invalidation 契约；首版只收集事实，不执行业务命令；
- 实现 SSH history/static/受控 command/service/path metadata completion 与 ghost overlay；
- 建立 Agent WS、Go canonical events、Vue `useChat`；
- 只开放 inspect/propose/explain，不发送 Enter。

### Phase 2：模型与 AI completion

- 接入 provider registry 和 OpenAI Responses adapter；
- 接入 `useObject` AI completion；
- 完成 secret redaction、budget、provider contract fixtures；
- AI 只建议，不执行。

### Phase 3：active PTY 只读命令

- 实现 BindingRegistry、AgentInputGateway、InputLease、readiness；
- 统一 CommandGuard/ReviewCoordinator；
- 实现版本化 ResolverRegistry、CommandIR/shell renderer、tool-specific output parser 和 digest chain；
- 完成 Ubuntu/Debian、RHEL 系、Alpine/OpenRC、SUSE 和 BusyBox/restricted-shell 的明确支持或 unsupported 矩阵；
- Review D-021，首批只开放 `inspect_system`、`inspect_disk`、`inspect_process`、`inspect_port`、`inspect_service` 和有限 `read_log`；
- 实现 running/unknown、inspect、takeover 和 interrupt。

### Phase 4：单步变更和单 session Runbook

- 精确审批、Core ticket、pre/post check；
- 先评估 `change_directory` 和 `manage_service`，再评估其他可逆低范围变更和预定义 rollback；
- 一个 Runbook 绑定一个 active terminal；
- 任一步 unknown/lease lost 时停止自动推进。

### Future：Task Agent（另立项）

只有明确需要后台续跑、多资产或定时时再设计。它不得成为 Terminal Agent fallback，必须有不同的 session、能力、执行和 UI 标识。

## 16. 上线门禁与指标

### 16.1 必测场景

1. 人工、共享用户和 Agent 命令都命中同一 Parser ACL/review；
2. 用户已有半条命令时 Agent 返回 `input_conflict`，不覆盖；
3. 两个 tool call、人类输入和跨节点迟到消息不会字节交叉；
4. old lease、stale revision、password、alternate screen、ZMODEM、review、pause、断线全部拒绝；
5. approval 参数修改、过期、重复 continuation 和跨 session resolve 全部失败；
6. action 前/accepted 后/written 后/record 前后故障不重复执行；
7. PS1 出现在普通输出、自定义 prompt、多行和长输出时不伪造 exit code；
8. Agent WS 重连恢复 event，不重放命令；
9. Take over 后 Agent write/interrupt 被服务端拒绝；
10. 输出进入模型前完成裁剪、脱敏和 untrusted 标记；
11. completion stale result、UTF-8/UTF-16/xterm cell 映射和 desync fallback 正确；
12. provider 首 event 后不 fallback，tool side effect 后不透明重试。
13. ToolRegistry 只下发当前版本允许的 SSH Tool；未知 name/version、模型伪造 Tool 和非 SSH Tool 全部拒绝；
14. service/path/PID/port/target/log filter 等参数覆盖 shell injection、unicode、长度、边界值和符号链接 fixtures；
15. 支持的 Linux distro/init system/shell resolver 使用 golden tests，`command_bytes_digest` 与 InputGateway 实际完整 submission bytes（含提交分隔符）一致；
16. 每个命令 Tool 都验证 timeout、max lines/bytes、redaction、ACL/review 和 unsupported-command 结果，且 unsupported 不改走任意 shell；
17. profile revision/TTL、reconnect、`su/sudo -i`、nested SSH、container/chroot、PATH/shell 变化和 `command not found` 会使旧 action stale；
18. Ubuntu/Debian、RHEL/Rocky/Alma/CentOS、Alpine、SUSE、BusyBox/restricted shell 覆盖 resolver + output parser golden fixtures；不支持组合返回明确错误；
19. locale、彩色/截断/恶意输出、flag 差异、parser failure 全部返回 bounded partial/unknown，不把解析失败解释成业务状态；
20. arguments/profile/action/approval/command bytes/raw output/model view 的域隔离 digest 不能跨类型替换；policy/profile/capability 任一 revision 变化都会拒绝旧 approval；
21. write 0 byte、部分写、完整写后断线分别产生可审计 receipt；部分写/unknown 不自动重发；
22. command preview 的空白、escape、Unicode 显示差异不能影响 exact bytes 校验，且任何 approval 后 payload mutation 都被拒绝。

### 16.2 关键指标

- duplicate remote execution：目标 `0`；
- cross-tenant/session leak：目标 `0`；
- execution ↔ command record correlation success；
- submit rejection reason 分布；
- lease contention 和 user takeover latency；
- unknown completion/exit-code ratio；
- profile unknown/stale rate、resolver coverage/unsupported rate、output parser unknown rate；
- action-to-write digest match、partial write 和 stale approval rejection；
- tool policy rejection/approval/timeout；
- completion p50/p95、coverage、acceptance、edit distance；
- model cost/token/latency 和 redaction hit；
- WebSocket replay/recovery success。

## 17. 开放问题

| ID | 问题 | 需要的证据 | 决策门点 |
| --- | --- | --- | --- |
| O-001 | 跨节点 submit 使用内部 RPC、Redis Stream 或其他可靠机制 | 故障注入、ack 延迟、部署复杂度、幂等测试 | Phase 3 前 |
| O-002 | shell integration 如何安全获取 start/end/PWD/exit code | Bash/Zsh 原型、兼容性与伪造风险 | Phase 3 前 |
| O-003 | `tmux/screen` 是否开放执行 | Pane/input snapshot 和命令归因 characterization | Phase 4 前 |
| O-004 | R0/R1 默认自动提交的组织级开关 | 安全评审和灰度数据 | Phase 3 上线前 |
| O-005 | CommandGuard 从 Parser 抽取的 Core API 边界 | 现有 ACL/review 回归测试 | Phase 3 前 |
| O-006 | provider/model 的默认部署与数据出境策略 | 组织配置、合规与成本评审 | Phase 2 前 |
| O-007 | LSP 是否带来足够增益 | 离线 completion eval 和运行成本 | Phase 2 后可选 |
| O-008 | D-021 首批六个 SSH 命令 Tool 的最终 Schema、resolver 和 distro 支持矩阵 | golden fixtures、ACL 回归、脱敏与 unknown-rate 原型 | Phase 3 前 |
| O-009 | profile bootstrap/shell integration 能可靠识别哪些上下文切换，TTL 如何分层 | Bash/Zsh/BusyBox/restricted shell、su/sudo/container/nested SSH 原型与 false-fresh 数据 | Phase 3 前 |
| O-010 | 审计存储是否需要 HMAC/event chain 或外部不可变存储 | 审计篡改威胁模型、密钥轮换、验证与归档成本 | 安全评审前 |

## 18. 已拒绝方案

| 方案 | 原因 |
| --- | --- |
| Terminal Agent 默认独立 SSH exec | 会话状态、身份、用户可见性和审计分裂 |
| active PTY 不可用时自动 fallback | 产生不可见第二执行面，可能重复副作用 |
| 浏览器/桌面客户端执行工具 | 凭据和状态不应由客户端掌握 |
| 模型风险标签决定 auto-run | 模型不是策略权威 |
| 裸 `Room.Receive` 作为 Executor | 无 ack、lease、revision、幂等和结果关联 |
| 根据 PS1 返回成功/exit 0 | prompt 可修改或出现在输出中，结论不可靠 |
| completion 隐藏命令写当前 PTY | 污染用户输入、录像和前台进程 |
| Node Agent runtime/sidecar | 与 Go-only 后端运行约束不一致 |
| 直接嵌入 AI Elements/React | 双框架状态、主题、体积和维护成本过高 |
| LSP 直接持有生产凭据 | 绕过 Koko metadata、权限和生命周期边界 |
| `sync.Map` 保存权威会话 | 无持久化、恢复和多节点一致性 |
| 在当前 Registry 顺带加入 K8s/数据库/SFTP Tool | 超出 SSH terminal 边界，权限、执行和结果语义不同，必须另立设计 |
| 按模型常识或资产 OS 标签直接生成 Linux 命令 | 无法反映当前 PTY 的 shell/init/PATH/container/nested SSH，审批对象不确定 |
| 在一个命令中用 `a || b` 探测并 fallback | 实际分支在审批前未知，输出 parser 和审计语义不唯一 |
| digest 相同就视为已授权或执行成功 | digest 只证明内容一致，不提供身份、权限、远端执行或输出真实性 |

## 19. 修订记录

| 版本 | 日期 | 内容 |
| --- | --- | --- |
| 0.1.0 | 2026-07-13 | 从 `terminal-ssh-agent-summary.md` 提炼正式设计；确定 active PTY、Go/Vue/AI SDK、provider、补全、安全、实施和 Review 规则 |
| 0.2.0 | 2026-07-13 | 将 Tool 范围收敛为 SSH/Linux terminal；补充固定后端 ToolDefinition、上下文/执行控制/只读/变更 Tool 清单、resolver 约束和明确拒绝项 |
| 0.3.0 | 2026-07-13 | 增加会话级 LinuxExecutionProfile、版本化 ResolverRegistry、CommandIR/output parser、stale/re-resolve 规则及域隔离 digest chain；说明 digest 的安全意义与边界 |

## 20. 证据与参考入口

- 研究与完整推导：[terminal-ssh-agent-summary.md](./terminal-ssh-agent-summary.md)
- active PTY 专项审查：[terminal-ssh-agent-summary.md §15](./terminal-ssh-agent-summary.md#15-active-pty-terminal-agent-设计审查与最终选择)
- Koko active bridge：`pkg/proxy/switch.go`
- Koko Parser/ACL/review：`pkg/proxy/parser.go`、`parsercmd.go`、`command_check.go`
- Koko Room/cross-node：`pkg/exchange/room.go`、`redis.go`、`redis_proxy.go`
- Koko server connection contract：`pkg/srvconn/conn.go`
- Warp 本地研究基线：`/opt/codes/warp@995e3dd7a2e16d5572db1cb24a9adbfadfe23da8`
