# Koko 内存观测

Koko 提供两种轻量观测方式，并保留 Go 自带的 pprof。返回值同时包含 Go 堆、进程 RSS、
cgroup 内存和终端解析器数量，用于区分 Go 对象与 `libghostty-vt` 的 native 内存。

## 即时快照

调试接口只允许从容器或主机的 loopback 地址访问：

```bash
curl -s http://127.0.0.1:5000/debug/memory | jq
```

主要字段：

- `go.heap_alloc_bytes`、`go.heap_inuse_bytes`：Go 当前堆；
- `go.sys_bytes`、`go.memory_limit_bytes`：Go 向系统申请的内存和 `GOMEMLIMIT`；
- `process.rss_bytes`：Koko 进程实际驻留内存，包含 Go、cgo/native、线程栈和映射页；
- `cgroup.current_bytes`、`cgroup.limit_bytes`：容器 cgroup 的当前使用量和限制；
- `terminal_parser.active`：当前存活的 `TerminalVT` 数量；
- `terminal_parser.scrollback_rows`：常驻解析器的滚动历史上限，当前固定为 0；
- `terminal_parser.output_buffer_limit_bytes`：单会话命令输出缓冲上限。

不方便访问 HTTP 端口时，可让进程把同一快照写入日志：

```bash
kill -USR1 <koko-pid>
docker kill --signal=USR1 <container>
kubectl exec <pod> -- kill -USR1 1
```

随后在 Koko、Docker 或 Pod 日志中搜索 `Memory snapshot:`。`SIGUSR1` 只采样，不会终止
服务。

## Go 堆分析

原有 pprof 同样只允许 loopback 访问：

```bash
go tool pprof http://127.0.0.1:5000/debug/pprof/heap
curl -o heap.pb.gz http://127.0.0.1:5000/debug/pprof/heap
```

pprof 的 heap profile 只统计 Go 堆，不包含 `libghostty-vt` 通过 cgo 分配的 native 内存。
判断 native 或容器侧增长时，应将 `process.rss_bytes`、`cgroup.current_bytes` 与 Go heap
一起观察。如果 RSS/cgroup 持续增长而 Go heap 稳定，应进一步检查 native 分配、线程栈
和页缓存。

## 容器运行的影响

容器不会改变终端解析逻辑。当前镜像静态链接 `libghostty-vt`，运行时不需要额外 `.so`；
但 native 内存仍计入进程 RSS 和容器 cgroup 限额。`GOMEMLIMIT` 只约束 Go runtime，无法
限制 cgo/native 内存，因此建议给 native 库、线程栈和系统组件预留约 20%–30% 的容器
内存，而不是把 `GOMEMLIMIT` 设置为容器 limit 的 100%。

常驻 `TerminalVT` 使用实际 PTY 窗口尺寸，并关闭 scrollback；窗口 resize 会同步调整
TerminalVT。命令输出另有每会话 100 KiB 的有界缓冲，因此窗口大小不再通过设置一个很大
的 scrollback 来兜底。
