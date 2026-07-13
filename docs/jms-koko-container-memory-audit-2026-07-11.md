# jms_koko 容器内存排查审计记录（2026-07-11）

## 范围与结论

排查对象为 Docker 容器 `jms_koko`（镜像 `jumpserver/koko:dev-ee`），主机使用
cgroup v2 与 systemd cgroup driver。本记录中的命令均为只读采集；未重启容器、未清理
页缓存、未修改 Docker 或应用配置。

采集时容器的 cgroup 内存约为 **119–123 MiB**，构成如下：

| 项目 | 观测值 | 结论 |
| --- | ---: | --- |
| `memory.current` | 约 119–123 MiB | cgroup 计费的总内存 |
| `memory.stat:anon` | 约 12–13 MiB，采样中基本稳定 | 应用匿名内存没有持续上涨证据 |
| `memory.stat:file` | 约 104 MiB | 主要增量来源是文件页缓存 |
| `memory.stat:active_file` | 约 68 MiB | 活跃文件缓存会被 Docker 工作集计入 |
| 主进程 RSS | 约 63–65 MiB | 与应用内存接口一致 |
| Go `heap_alloc_bytes` | 约 7 MiB | 未见 Go 堆异常增长 |
| 主进程 `VmSwap` | 0 | 未发生 swap |

`/proc/<pid>/exe` 指向的 `/opt/koko/koko` 大小为约 118 MiB，其中约 72.5 MiB 在页缓存中。
该镜像的 Koko 可执行文件较大（静态链接），它和运行中读过的日志、回放文件均会使
`memory.stat:file` 增加。应用监控通常报告 Go 堆或进程 RSS，**不会等同于 cgroup 的
文件页缓存**；因此“应用内存正常、容器内存上涨”在当前样本中属于统计口径差异与页缓存
增长，尚无应用内存泄漏证据。

Docker 在 cgroup v2 下的 `stats` 工作集可近似理解为：

```text
docker working set ~= memory.current - inactive_file
```

因此它也不等于应用进程 RSS。内核可在有内存压力时回收可回收的文件缓存；不应通过
`drop_caches` 作为常规处理手段，它会影响全主机缓存并掩盖问题。

## 当时业务关联

Koko 日志显示 SSH 会话、录像 `.cast` 创建/压缩/上传活动；采样期间主进程几乎无磁盘读，
仅有少量日志/录像写入。绑定数据目录中 `koko.log` 约 1.8 MiB，不能单独解释约 104 MiB
的文件缓存；大头来自镜像可执行文件及已访问的镜像/文件页。

## 已执行的采集命令

以下命令避免输出容器完整环境变量（其中可能含凭据）。执行前以实际容器名替换
`jms_koko`；需要读取宿主机 `/proc`、`/sys/fs/cgroup` 的权限。

### 1. 容器、限制和 Docker 视图

```bash
docker version --format '{{.Server.Version}}'
docker ps --filter name='^/jms_koko$' \
  --format 'table {{.Names}}\t{{.ID}}\t{{.Status}}\t{{.Image}}'
docker stats --no-stream \
  --format 'table {{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.CPUPerc}}\t{{.PIDs}}' \
  jms_koko
docker inspect jms_koko --format \
  '{{json .HostConfig.Memory}} {{json .HostConfig.MemorySwap}} {{json .HostConfig.MemoryReservation}} {{json .HostConfig.PidsLimit}} {{json .State.Pid}}'
docker info --format 'CgroupDriver={{.CgroupDriver}} CgroupVersion={{.CgroupVersion}} MemoryLimit={{.MemoryLimit}}'
```

本次：未设置 Docker 内存限制（`memory.max=max`），容器有 11 个进程/线程组成员，
`memory.events` 的 `oom`、`oom_kill`、`high`、`max` 均为 0。

### 2. 定位 cgroup v2 并读取内存构成

```bash
pid=$(docker inspect -f '{{.State.Pid}}' jms_koko)
cg=$(awk -F: '$1=="0" {print $3}' /proc/$pid/cgroup)
base=/sys/fs/cgroup$cg

cat "$base/memory.current"
cat "$base/memory.max"
cat "$base/memory.high"
cat "$base/memory.events"
cat "$base/memory.stat"
cat "$base/io.stat"
cat "$base/pids.current"
```

重点比较 `anon`、`file`、`shmem`、`kernel`、`active_file`、`inactive_file`，而不是只看
`memory.current`。若 `anon` 或 `shmem` 连续增长，才优先沿应用内存/共享内存方向排查；
若主要是 `file`，优先排查文件读取、镜像层和日志/录像等 I/O。

### 3. 60 秒趋势采样

```bash
pid=$(docker inspect -f '{{.State.Pid}}' jms_koko)
cg=$(awk -F: '$1=="0" {print $3}' /proc/$pid/cgroup)
base=/sys/fs/cgroup$cg
printf '%-10s %12s %12s %12s %12s %12s\n' time current anon file active_file inactive_file
for i in 1 2 3 4 5 6; do
  stat=$(cat "$base/memory.stat")
  value() { awk -v k="$1" '$1 == k { print $2 }' <<<"$stat"; }
  printf '%-10s %12s %12s %12s %12s %12s\n' "$(date +%T)" \
    "$(cat "$base/memory.current")" "$(value anon)" "$(value file)" \
    "$(value active_file)" "$(value inactive_file)"
  sleep 10
done
```

本次短时采样中 `anon` 仅在约 12–13 MiB 范围内波动，`file` 约 104 MiB，支持“页缓存为主”的判断。

### 4. 应用进程与 Koko 自身快照

```bash
pid=$(docker inspect -f '{{.State.Pid}}' jms_koko)
awk '/^(Name|Pid|PPid|Threads|VmRSS|RssAnon|RssFile|RssShmem|VmSwap):/ {print}' /proc/$pid/status
awk '/^(Rss|Pss|Pss_Anon|Pss_File|Pss_Shmem|Anonymous|Swap):/ {print}' /proc/$pid/smaps_rollup
ps -eo pid,ppid,rss,vsz,%mem,etimes,comm,args --sort=-rss | awk -v root="$pid" '$1==root || $2==root {print}'
docker exec jms_koko sh -c 'curl -fsS --max-time 3 http://127.0.0.1:5000/debug/memory'
```

最后一条访问的是容器 loopback 的只读诊断接口。接口不可用时可采集快照日志：

```bash
docker kill --signal=USR1 jms_koko
docker logs --tail 200 jms_koko | grep 'Memory snapshot:'
```

`SIGUSR1` 会让应用写出采样日志；执行前应确认当前版本支持该信号。Go pprof 仅用于分析
Go 堆，不能解释 cgroup `file`：

```bash
docker exec jms_koko sh -c 'curl -fsS http://127.0.0.1:5000/debug/pprof/heap' > heap.pb.gz
go tool pprof heap.pb.gz
```

### 5. 将文件缓存关联到文件和 I/O

```bash
pid=$(docker inspect -f '{{.State.Pid}}' jms_koko)
fincore --bytes /proc/$pid/exe
stat -Lc '%s %n' /proc/$pid/exe
find /data/jumpserver/koko/data -xdev -type f -printf '%s %p\n' | sort -nr | head -20
find /data/jumpserver/koko/data -xdev -type f -printf '%s %p\n' | sort -nr | head -20 \
  | cut -d' ' -f2- | xargs -r fincore --bytes
awk '/^(rchar|wchar|syscr|syscw|read_bytes|write_bytes|cancelled_write_bytes):/ {print}' /proc/$pid/io
pidstat -d -p "$pid" 1 3
```

工具说明：`fincore`（util-linux）显示指定文件的页缓存驻留量；`pidstat`（sysstat）观察
进程 I/O；`/proc/<pid>/io` 是进程累计 I/O 计数。若未安装这些工具，应在变更单中审批后
安装 `util-linux`/`sysstat`，不要在生产排查时临时使用不受控脚本。

## 后续判定与处置门槛

1. 每 10–60 秒采样并至少覆盖一次高并发会话、回放上传或大文件传输。只要 `anon`、RSS、
   Go heap 不随时间单调增长，而 `file` 增长，按页缓存观察即可。
2. 若 `anon`、`RssAnon` 或 `heap_alloc_bytes` 连续增长，保留两份间隔 5–10 分钟的
   `heap.pb.gz`，用 `go tool pprof -base old.pb.gz new.pb.gz` 比较；同时记录会话数与版本。
3. 若 `file` 持续增长并影响同机工作负载，先定位读取源（回放、日志、上传下载或镜像层），
   再评估日志轮转、回放存储策略和容器合理的 `--memory`/`--memory-reservation`。设置限制前
   应预留内核、文件缓存和业务峰值空间，并观察 `memory.events` 的 OOM 计数。
4. 禁止把 `echo 3 > /proc/sys/vm/drop_caches` 当作修复方案；它是全主机、破坏缓存命中的
   临时操作，且不会解决匿名内存泄漏。
