# 复测说明

## 本地微基准

本轮基准在 Go 1.23.1 / darwin arm64 / Apple M3 Max 上记录，基准 GOMAXPROCS=1。go.mod 现已升级到 Go 1.27.1，下列命令按当前发布工具链执行，重跑的绝对数值会与记录不同。不要并行运行编译、race 和计时基准；先编译测试二进制，再交替执行 before/after。结果取 5 次中位数，没有生产负载置信区间。

```sh
GOTOOLCHAIN=go1.27.1 GOMAXPROCS=1 go test ./common/limiter ./common/rule ./service/controller -run '^$' -bench=. -benchmem -benchtime=200ms -count=5
GOTOOLCHAIN=go1.27.1 GOMAXPROCS=1 go test ./app/mydispatcher -run '^$' -bench=BenchmarkDispatcherGetLink -benchmem -count=5
GOTOOLCHAIN=go1.27.1 go test -race ./common/limiter ./common/rule ./app/mydispatcher ./service/controller -run '^Test(Bucket|Rule|Dispatcher|UserDiff)' -count=1
GOTOOLCHAIN=go1.27.1 go vet ./common/limiter ./common/rule ./app/mydispatcher ./service/controller
```

`BenchmarkBucketFirstConnection` 包括删除两张 map 项，以便下一轮重新首次创建，不能将其 ns/op 当纯建连时间。`BenchmarkBucketParallel` 用同一用户/IP 制造竞争；需 `GOMAXPROCS=2` 或更多 CPU 才是实际并行测量。用户 diff 的字段完全一致、逆序、1% 变更、100% 变更各自测试，不把相同顺序的快路径当所有 diff 的结果。

before 来自提交 `0bb4713626128849da0d2437defea1a1322c44cc`；测试文件使用本次相同版本。可以创建该提交的独立 worktree，复制以下新增测试后运行相同命令：

- common/limiter/limiter_test.go
- common/rule/rule_test.go
- service/controller/userdiff_test.go
- app/mydispatcher/performance_test.go

本次复测使用 Go `-overlay` 将三个业务文件映射到 `git show` 导出的原始版本，未反复改写工作区。该工具链 overlay 与当前 import slices 的 vet 输入存在不匹配，基准测试二进制编译需 `go test -c -vet=off -overlay=...`；正式修改后 vet 独立执行，不以此跳过验收。

固定操作量 GC 与存活堆：

```sh
GOTOOLCHAIN=go1.27.1 GOMAXPROCS=1 go test ./common/limiter -run '^Test(BucketAllocationWorkload|LimiterRetainedMemory)$' -v -count=1
GOTOOLCHAIN=go1.27.1 GOMAXPROCS=1 go test ./app/mydispatcher -run '^TestDispatcherRetainedLinks$' -v -count=1
```

用户状态的 heap delta 包含 fixture key，未包含 Panel 列表和 Xray 认证状态。getLink 留存实验只创建 pipe/wrapper，不创建 socket/握手/dispatch goroutine；该内存不是完整 active connection 成本。

Profile 示例（输出到工作区外）：

```sh
GOTOOLCHAIN=go1.27.1 GOMAXPROCS=1 go test ./common/limiter -run '^$' -bench '^BenchmarkGetUserBucket$/^users=1000$/^tcp=true$' -benchtime=2s -cpuprofile=/tmp/limiter.cpu -memprofile=/tmp/limiter.heap -o /tmp/limiter.test
GOTOOLCHAIN=go1.27.1 go tool pprof -top /tmp/limiter.cpu
GOTOOLCHAIN=go1.27.1 go tool pprof -top -alloc_space /tmp/limiter.heap
GOTOOLCHAIN=go1.27.1 GOMAXPROCS=2 go test ./common/limiter -run '^$' -bench '^BenchmarkBucketParallel$' -benchtime=2s -mutexprofile=/tmp/limiter.mutex -blockprofile=/tmp/limiter.block -o /tmp/limiter.test
GOTOOLCHAIN=go1.27.1 go tool pprof -top /tmp/limiter.mutex
GOTOOLCHAIN=go1.27.1 go tool pprof -top /tmp/limiter.block
```

微基准 profile 包括初始化/校准，所以 after 无每次分配不代表整个 profile 没有分配。不要跨不同操作次数比较 profile 的累计 MB 来计算收益。CPU profile 中 macOS kevent 采样不能外推 Linux syscall CPU。

## 全仓库验证边界

API 的原有测试连接 localhost Panel 并会 POST 流量/在线数据；TestController 会等待操作系统信号；LegoDNSCert/HTTPCert/RenewCert 使用真实 ACME。未提供对应服务和证书环境，不能声称原样 `go test ./...` 的全部外部测试已运行通过。本次未改这些测试来隐藏问题，执行的是显式跳过外部集成测试的全包检查：

```sh
GOTOOLCHAIN=go1.27.1 go test ./... -skip '^Test(Get|Report|Controller$|LegoDNSCert$|LegoHTTPCert$|LegoRenewCert$)' -timeout=60s -count=1
GOTOOLCHAIN=go1.27.1 go test -race ./... -skip '^Test(Get|Report|Controller$|LegoDNSCert$|LegoHTTPCert$|LegoRenewCert$)' -timeout=60s -count=1
GOTOOLCHAIN=go1.27.1 go vet ./...
```

剩余原有失败：TestBuildSS 的 CypherMethod 为空；vet 的 PMPanel 测试 DetectResult 无字段名构造。staticcheck 2024.1.1 也报告原有问题，完整日志在 results 中。Linux 交叉构建命令：

```sh
GOTOOLCHAIN=go1.27.1 GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -o /tmp/XrayR-linux-amd64 .
```

## Linux Profile A/B/C 下一步实测

使用隔离测试 Panel、受控客户端与自有 echo/下载服务；不要向生产 Panel 发送合成计费流量。两版使用相同 Go 版本、协议、TLS、规则、证书、BufferSize 和流量种子。A=1CPU/512MiB/100用户；B=1CPU/1GiB/1000用户；C=2CPU/1GiB/5000+用户。先记录资源限制和 swap，再执行以下矩阵：

1. 无活跃连接预热 60 秒，记录 idle RSS 与 heap。
2. 相同配置分别加载 100/1000/5000/10000 用户，GC 后求 heap/RSS 斜率。
3. 以 100/1000/5000 并发运行长连接 5 分钟，再单独测短连接 CPS；TCP 和 UDP 分开，开启/关闭限速分别测。冷建连、相同 IP 重连、不同 IP、新旧在线周期边界都覆盖。
4. 以 A 的 100–500Mbps、B 的 500Mbps–1Gbps、C 的 1Gbps+ 为目标阶梯增加负载，记录实际达到吞吐，不将目标当承诺；同时确认上下行计费与服务端实际字节。
5. 注入 Panel 超时/错误、用户增删改、节点热更新、Redis 不可用，核查错误率、未报告流量、goroutine 回落、历史 map 留存。
6. 停止负载后等两轮同步，检查 RSS/heap/goroutine 是否回落；历史 IP/用户 map 长期增长需要至少数小时 churn 测试。

Linux 只读采样（按实际测试进程 PID 替换）：

```sh
pidstat -u -r -w -p PID 1
cat /proc/PID/status
cat /proc/PID/smaps_rollup
ss -s
```

CPU/Gbps = 进程 CPU cores consumed / 实际 Gbps；RAM/user 和 RAM/connection 用多个规模的增量斜率，不能拿总 RSS 直接除用户数。syscall 计数可以在测试机用 `perf stat -p PID` 或短时 `strace -c -p PID`，但 trace 会扰动性能，需与无 trace 的吞吐结果区分。

本项目未暴露 HTTP pprof，不能直接假设 localhost:6060 可用。生产式完整 profile 需在隔离测试 harness 中启动同一 core/Controller，用 runtime/pprof 写文件、runtime.ReadMemStats 记录 HeapAlloc/HeapObjects/NumGC、runtime.NumGoroutine 记录协程；mutex/block profiling 只在采样阶段开启。本次未加入常驻管理端口。

## Runtime 与构建建议

先保持 GOGC 默认 100，依据实测活堆和 RSS 决定 GOMEMLIMIT；不要把 512MiB 全部给 Go，需扣除 OS、socket buffer、其他进程及 Go 未计入的内存。GOMEMLIMIT 是软限制，过低可能造成 GC 频繁工作，不能作为 OOM 的硬保证。GOMAXPROCS 对齐实际 CPU 配额后 A/B 测试，不在代码中硬编码。依据：[Go GC Guide](https://go.dev/doc/gc-guide)。

本轮保留所有 core 注册项与编译能力。减 binary 优先评估 lego DNS provider/可选协议的构建组合，但必须设计明确 build tags、验证各协议与默认功能全集；没有证据前不删除依赖。剥离符号优化的是文件大小，不等于减少连接堆内存。
