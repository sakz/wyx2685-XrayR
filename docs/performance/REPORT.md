# Performance Report

2026-09-06，基准提交 `0bb4713626128849da0d2437defea1a1322c44cc`，分支 `codex/performance-audit`。

完成本地系统审计、三项可测量的行为保持优化及相关回归验证。未部署；尚不能据此承诺 1 核 VPS 的并发/吞吐容量或整机 CPU/RSS 降幅。没有修改 core、Panel 协议、计费策略、限速规则、runtime 参数或依赖。

- [完整调用链、数据流、goroutine、timer、共享数据与锁图](AUDIT.md)
- [全部规模的 benchmark 中位数](results/SUMMARY.md)
- [原始 baseline](results/before.txt)、[limiter after](results/after-limiter.txt)、[diff after](results/after-diff.txt)、[rule after](results/after-rule.txt)
- [profile 分析](results/PPROF.txt)、[可复测命令与 Linux A/B/C 实验](REPRODUCE.md)

## 修改前 / 修改后

同一 Go 1.23.1、Apple M3 Max、darwin/arm64，GOMAXPROCS=1。以下耗时取 5 次中位数；dispatcher 和首次连接以预编译测试二进制交替复测，避免编译干扰。微基准不是 Linux VPS 端到端吞吐测试。

| 指标/场景 | 修改前 | 修改后 | 变化 |
| -- | --: | --: | --: |
| 整机 RSS / CPU% / CPU per Gbps | 未测 | 未测 | 无可报告比例 |
| limiter 1000 用户重复 TCP，ns/op | 558.1 | 115.1 | -79.4%，约 4.85 倍 |
| 同场景 B/op | 504 | 0 | -100% |
| 同场景 allocs/op | 14 | 0 | -100% |
| dispatcher getLink + close，ns/op | 1552 | 1137 | -26.7% |
| dispatcher B/op / allocs/op | 2096 / 48 | 1592 / 34 | -24.0% / -29.2% |
| 50000 用户、不变列表 diff，ns/op | 38,125,840 | 342,447 | -99.1%，约 111 倍 |
| 同场景 B/op / allocs/op | 56,284,176 / 4030 | 0 / 0 | -100% |
| 50000 用户、1% 变更 diff，ns/op | 39,071,430 | 14,055,456 | -64.0% |
| 同场景 B/op | 56,596,666 | 7,946,560 | -86.0% |
| 50000 用户、全变更 diff，ns/op | 60,901,948 | 36,646,868 | -39.8% |
| 同场景 B/op | 96,971,216 | 73,392,557 | -24.3% |
| 10000 用户、逆序 diff，ns/op | 7,395,651 | 2,351,161 | -68.2% |
| 20 规则未命中，ns/op | 974.3 | 463.1 | -52.5% |
| 同场景 B/op / allocs/op | 480 / 20 | 0 / 0 | -100% |
| 20 规则最后一条命中，ns/op | 1364 | 747.6 | -45.2% |
| 同场景 B/op / allocs/op | 800 / 27 | 64 / 2 | -92.0% / -92.6% |
| 首次 bucket 创建+清理，ns/op | 985.1 | 961.5 | 约 -2.4%，视为基本持平 |
| 同场景 B/op / allocs/op | 1104 / 24 | 1104 / 24 | 无变化 |

100/1000/10000/50000 用户、TCP/UDP、diff 不同变更率均保留原始结果。没有仅挑单个数据点宣称整体提升。初次 dispatcher after 计时受并行编译污染，`before-dispatcher.txt` / `after-dispatcher.txt` 的计时不用于结论；B/op、固定工作量 GC 和留存堆仍可读，计时采用 `recheck-before.txt` / `recheck-after.txt`。

## GC、Heap 和 goroutine

1000 用户已创建状态，固定执行 **1,000,000 次重复 TCP 检查**，5 次运行：

| 指标 | 修改前中位数 | 修改后中位数 |
| -- | --: | --: |
| TotalAlloc 增量 | 504,016,608 B | 16,032 B |
| Mallocs 增量 | 14,001,002 | 1,002 |
| NumGC 增量 | 171 | 0 |
| GC pause 累计 | 4,152,047 ns | 0 |

此场景累计分配减少约 99.997%，不是全进程或全负载 GC CPU 下降比例。after 的少量一次性分配包含 sync.Map 初期读表提升，稳定命中 benchmark 为 0 alloc/op。未调整 GOGC/GOMEMLIMIT，未用 sync.Pool。

留存堆实验显示这三项优化主要减少**短生命周期对象**，没有降低长期用户状态和连接包装结构体大小：

| 1000 用户 fixture | 修改前 | 修改后 |
| -- | --: | --: |
| configured limiter HeapAlloc 增量 | 202,272 B | 202,272 B |
| configured HeapObjects 增量 | 6018 | 6018 |
| 每用户一 IP + 限速 bucket 的额外 HeapAlloc | 626,248 B | 626,760 B |
| 上述额外 HeapObjects | 14503 | 14507 |
| goroutine 增量 | 0 | 0 |
| 1000 个空 getLink 的 HeapAlloc 增量中位数 | 1,375,592 B | 1,375,592 B |
| 空 getLink goroutine 增量 | 0 | 0 |

fixture 包含测试持有的字符串 key，未包含 Panel 用户列表、core 认证和真实 socket。getLink 实验没有调用 routedDispatch，因此 0 只表示包装层本身未新增 goroutine；实际代理 dispatch 每调用仍有一条任务及上游协议协程。不要据此宣称整机 RAM 或 goroutine 数量减少。50000 用户 fixture 出现 -1 goroutine 是运行时背景任务波动，不是优化回收了一个协程。

## 为什么保留这些修改

| 优化 | 原因/证据 | 修改 | 性能提升 | 风险与验证 |
| -- | -- | -- | -- | -- |
| limiter 候选对象按需创建 | allocs profile 中临时 sync.Map.Store/Swap、rate.NewLimiter 是主分配来源 | 先 Load，miss 才构造并 LoadOrStore；IP 命中先读 | 重复 TCP 检查约 4.85 倍，dispatcher 路径 -26.7% | 没有改 map 类型/锁/限速值；并发首次连接共享同一 bucket、设备分支、在线周期、更新速率、UDP 回归与 race 通过 |
| 用户 diff 减少索引和交集 | 大列表原算法每轮分配约 56–97MB；profile 分配集中在 compareUserList | 相同列表直接返回；其余用一张预分配状态 map | 不变列表约 111 倍；1% 变更约 2.78 倍；全变更约 1.66 倍 | 仍比较全部 UserInfo 字段，保留原重复输入语义；2000 组随机输入与旧算法对照，不修改输入 |
| 规则 MatchString / 命中结果复用 | 每规则转 []byte，每命中构造候选 set | MatchString，已有结果先 Load，miss 仍 LoadOrStore | 20 规则未命中约 2.10 倍，末规则命中约 1.82 倍 | 保留首规则优先、拒绝/记录逻辑、非法 UID 行为与去重；并发首命中和 race 通过 |

没有新增长期缓存、goroutine、buffer、全局锁或依赖。原本不在线用户仍按原规则清 bucket。首次连接测量基本持平，没有用热点收益掩盖明显冷路径退化。

## Profile 结论与证据边界

- limiter before 的 alloc_space：sync.Map.Swap 累计约 60.8%、rate.NewLimiter 约 16.4%；after 稳态 CPU 主要是 mapaccess2 / sync.Map.Load / hash。累计 profile 包含初始化和 benchmark 校准，不能直接拿 before/after GB 差计算收益。
- 大型不变列表 before 的 alloc_space 约 98% 在 compareUserList；after 该函数零分配，profile 剩下测试数据构造及依赖 init。
- GOMAXPROCS=2 同用户并行 probe：268.2 → 38.33 ns/op，504B/14 → 0B/0。只有一次带 profile 的采样，不取代五轮主基准。mutex profile 主要是 runtime 内部 contention，block profile 主要是测试 WaitGroup/channel；**没有足够证据宣称业务全局锁是生产最大瓶颈**。
- 逃逸分析确认新 map/rate.Limiter 在 miss 时仍需逃逸，符合共享状态生命周期；GetUserBucket 超出 inline 预算。本次通过减少执行分配的频率改善，没有强行拆函数或改用 unsafe。
- Linux amd64 CGO=0、trimpath、原 ldflags 构建成功；产物约 117MiB。不是 Linux 启动/吞吐/协议兼容验证，二进制体积也不是 RSS。

## 正确性与验证结果

| 检查 | 结果 |
| -- | -- |
| 新增 limiter / rule / diff / dispatcher 回归 | 通过 |
| 独立代码复核 | 未发现本次新增明显正确性、并发或兼容性问题；审阅者另跑定向回归通过 |
| 上述四包选定测试的 race | 通过，无报告的新 race |
| 修改相关四包 go vet | 通过 |
| Linux amd64 交叉构建 | 通过 |
| 全包本地测试（明确跳过外部集成） | 未全绿：原有 TestBuildSS 漏填 CypherMethod |
| 全包本地 race（相同跳过范围） | 相同 TestBuildSS 失败；不能称全套通过 |
| go vet ./... | 原有 PMPanel 测试两处无字段名 DetectResult 构造 |
| staticcheck 2024.1.1 ./... | 原始源码导出版本 19 项、修改后 18 项，无新增诊断；减少的一项是被替换旧 diff 中的无效赋值 |
| 原样 go test ./... / go test -race ./... 全部外部集成 | 未执行：需要真实 Panel/ACME，且 TestController 等待信号 |
| Linux Profile A/B/C、真实协议端到端、生产 RSS/CPU | 未执行，缺少指定环境和负载 |

此次新增测试没有证明原项目所有并发路径无 race。审计确认的既有风险包括：上报成功 Set(0) 会清掉 HTTP 期间新增流量；nodeInfo/userList/AliveList 缺少跨任务同步；Periodic.Close 不等待进行中回调；用户删除未清理 limiter 用户表、历史 IP 未设置清理边界；全局 Redis 缓存写入无界 goroutine、删除节点缺少显式 client.Close。它们影响可靠性与长期内存，不能因局部 benchmark 通过而忽略。详见 AUDIT.md。

## 最后的八个问题

1. **最大 CPU 瓶颈？** 生产未测，无法给全进程排名。已测 XrayR 局部路径中，大用户同步的 diff 在原版最昂贵；优化后的连接限制 CPU 主要花在多级 map/hash。真实流量的加密/relay 与 Panel JSON 成本需要完整 CPU profile 排名。
2. **最大 RAM 消耗？** 没有真实 heap/RSS 无法确认。代码上的主要规模项是 core 用户认证、活动连接/pipe/协议缓冲、Panel 完整用户列表及在线/IP 表。测量已证实原版 50000 用户 diff 的单轮临时分配可达约 97MB；这不是 peak RSS。长期用户/IP 残留是另一风险。
3. **最大 GC 来源？** 在已测重复建连中为临时 IP map 和 bucket，在大用户同步中为两张索引及交集切片。两者已明显减少；全进程剩余最大来源需把 JSON/真实 relay 纳入 profile。
4. **最大锁竞争？** 无生产证据。候选是同用户 rate.Limiter Mutex、全 core stats.Manager RWMutex，以及 SSPanel/BunPanel 持锁 HTTP 阻塞用户解析；本次局部并发 profile 没有显示显著业务大锁等待，不进行猜测性 sharding。
5. **RAM/user？** 当前 limiter fixture 约 187–209B/配置用户（含 fixture key）；一个在线 IP 加限速 bucket 再增加约 591–627B/活跃用户。这不是完整 XrayR RAM/user，不能用它给 VPS 容量定额。
6. **RAM/active connection？** getLink 空 pipe/wrapper 约 1.38KB/连接，前后相同；不含 socket、payload、TLS、协议状态、goroutine 栈、内核 buffer。完整连接成本未知，须按真实协议和积压程度实测增量斜率。
7. **架构最大的扩展性瓶颈？** 用户同步与统计仍是每节点 O(N)，大型 JSON、完整快照和 churn 状态保留会形成内存峰值；可选全局 IP 限制把 cache/Redis 引入建连路径且异步写无界。最终哪项先到极限取决于 CPS、吞吐、用户量、节点数和配置。
8. **下一阶段最值得的三项？** ① 先为流量 snapshot/确认扣减和控制器退出/快照同步建立故障与 race 回归；② 在真实 Panel 样本下测 typed JSON 与周期统计，验证减少重复编解码、全表扫描和持锁 HTTP 的可行性；③ 用 Linux A/B/C 长时间 churn/高并发测试确定用户/IP/Redis 生命周期及每连接内存，再决定有界异步写或状态清理设计。

这些建议不等同于本轮已实现的优化。下一阶段的生产式 profile 和正确性门槛通过前，不建议将本报告解释为“可直接上线、容量已达标”。
