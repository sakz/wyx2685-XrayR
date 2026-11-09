# Performance Report — Round 2

2026-10-25，分支 `codex/performance-audit`，基于 `04c0946`（第一轮优化）之上。
本轮目标：弱配置 VPS 上服务更多用户（内存）、压榨带宽与提升单用户速度（CPU/延迟）。

范围分两层：**部署层**（不改代码）与**代码层**（6 项修改）。全部本地验证；未在真实 Linux VPS
上做端到端吞吐测试，容量结论仍以 [REPRODUCE.md](REPRODUCE.md) 的 Linux 实验为准。

## 部署层（新增产物）

| 文件 | 内容 |
|---|---|
| `release/config/99-xrayr.conf` | sysctl：BBR+fq、按 BDP 放宽 TCP 缓冲、tcp_notsent_lowat、somaxconn 等 |
| `release/config/xrayr.service` | systemd 单元：GOMEMLIMIT/GOGC（用内存换 CPU）、LimitNOFILE |
| `docs/performance/DEPLOYMENT.md` | 部署指南：内核/runtime/config.yml 调优与协议选型建议 |

config.yml 侧建议（见指南）：内存紧张降 `BufferSize` 到 32、无域名规则时 `DisableSniffing`、
大用户量提高 `UpdatePeriodic`、配置带缓存的 DNS。这些不改默认值，由部署者按需选择。

## 代码修改

| # | 修改 | 文件 | 动机 |
|---|---|---|---|
| 1 | 删除用户清理 limiter 残留 | `common/limiter/limiter.go` `DeleteUserLimiter`；`service/controller` 接线 | 原实现删认证后 UserInfo/BucketHub 条目永久残留，churn 大的站点内存单调增长 |
| 2 | OldUserOnline TTL 淘汰 | `common/limiter/limiter.go` | 历史 IP 只在"同 IP 重连"时删除，否则无限增长；现在 15 分钟未再出现的条目在 `GetOnlineDevice` 收集时过期 |
| 3 | 流量上报改差值制 | `service/controller/control.go` `deductTraffic`；`controller.go` 监控循环 | 原 `Set(0)` 会清掉 HTTP 上报期间新增的流量（慢面板/弱 CPU 下丢计费字节）。改为上报后 `Add(-已上报值)`，读前扣后，上报期间新增流量自动滚入下一周期；失败保留。计数器变累计制，int64 字节溢出不可达 |
| 4 | counter/policy 缓存 | `app/mydispatcher/default.go` | 原每连接 `GetOrRegisterCounter`×2（stats.Manager 全局锁）+ `ForLevel` 锁 + 字符串拼接；每周期每用户 2 次同样开销。现在 sync.Map 缓存，重复连接 0 锁 0 拼接；用户删除/节点重建时注销（`RemoveUserCounters`/`RemoveTagCounters`），顺带修复 stats manager 中 churn 用户的 counter 泄漏 |
| 5 | RateWriter 修复 | `common/limiter/rate.go` | ① `WaitN` 用 `context.Background()`，连接关闭后 goroutine 仍挂等到 token；改传连接 ctx，取消即返回。② `WaitN` 错误被忽略：写块大于 burst（低速率用户 <512kbps）时限速直接失效放行；现分块等待正确执行限速 |
| 6 | v2board 用户列表单次 typed 解码 | `api/newV2board/v2board.go` `GetUserList` | 原 simplejson 全量解析→re-Encode users→二次 typed Unmarshal；三遍开销随用户数放大。现在一次 `json.Unmarshal` 进 `struct{Users []*user}`。错误分支顺序与原 parseResponse 一致（err→status→解析） |

已知语义变化（有意为之）：
- #3 计数器不再归零，改累计+扣减；上报数值口径不变（每周期增量），失败重试不丢流量。
- #4 仅对真正删除的用户注销 counter/limiter 状态。属性变更（限速、设备数、凭据）在
  `compareUserList` 中表现为 delete+add，但用户 tag 不变，状态保留，存量连接的流量继续计入；
  节点 tag 不变的重建同样不再清空 counter。
- #5 限速低于 512kbps 的用户原先实际未受限，现在真正受限——这是修复而非退化。

## 审查补丁（review 发现的问题修复）

| # | 修复 | 文件 | 说明 |
|---|---|---|---|
| P1 | `UserCounters` 注册竞态 | `app/mydispatcher/default.go` | 两个 goroutine 并发首次注册同一 counter 时，输家的 `RegisterCounter` 返回 `(nil, "already registered")`，旧代码丢弃 error 并把 nil 存入缓存 → 该用户流量**永久**不再统计上报。现在丢失注册时回退 `GetCounter` 取回赢家的 counter，且含 nil 的 pair 一律不缓存（下条连接可重试） |
| P3 | `waitN` burst 收缩竞态 | `common/limiter/rate.go` | `UpdateInboundLimiter` 调小 burst 与分块等待并发时，`WaitN` 返回 "n exceeds limiter's burst" 硬错误断开连接。现在逐块重读 burst，等待失败且 ctx 存活时重试（唯一非 ctx 失败源就是 burst 检查，重读后收敛） |
| P1 | 属性变更误删 counter | `service/controller/controller.go` `splitDeletedUsers` | 限速/设备数/凭据变更会走 delete+add，但 tag 不变；原逻辑注销 counter 后存量连接流量不再上报。现只清理真正消失的用户 |
| P2 | 同 tag 重建清空 counter | `service/controller/controller.go` `nodeInfoMonitor` | tag 未变时不注销 inbound counter，避免重建后全部存量连接漏报 |
| P3 | `.workbuddy/` 入库 | `.gitignore` | 本地工具状态目录加入忽略清单 |

新增回归测试：`TestUserCountersLostRegistrationRace`（mock 输家注册序列，旧实现必失败）、
`TestUserCountersNoCacheOnNil`、`TestWaitNSurvivesBurstShrinkRace`（热震荡收缩压测，旧实现必失败）、
`TestWaitNThrottles`（3× burst 节流时长校验）、`TestSplitDeletedUsers`（属性变更用户不清理）。

## 验证

| 检查 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go vet ./...` | 仅剩基线已有的 PMPanel 测试 2 处 unkeyed fields 告警 |
| limiter / rule / mydispatcher 全部测试 + `-race` | 通过（含新增 TestDeleteUserLimiter、TestOldOnlineExpiry、TestUserCountersCache、TestRemoveTagCounters、TestDeductTraffic） |
| controller 包单测（diff/流量扣减）+ `-race` | 通过；TestController 为需真实面板的集成测试，基线同样失败 |
| 全量 `go test ./...` 失败集合 vs 基线 | 完全一致（api/*、mylego、TestController 均为需外部服务的集成测试，connection refused） |
| Linux amd64 CGO=0 交叉编译（原 ldflags） | 通过，约 116MB |
| BenchmarkGetUserBucket（重复 TCP 检查） | 1000 用户约 81ns/op、0 B/op、0 allocs/op（保持第一轮水平） |
| BenchmarkDispatcherGetLink（getLink+close） | 约 700ns/op、1464B/op、32allocs/op（第一轮 after 为 1137ns、1592B、34allocs，同机） |

## 遗留

- `pushIP` 无界 goroutine、Redis client 未显式关闭（仅启用 Redis 全局设备限制时相关）未动，留待下一轮。
- Xray-core 仍为 1.8.25-dev 快照；升级与 Linux splice 实测（`CanSpliceCopy=3` 是否走通）是下一阶段最大潜在收益。
- 修改尚未提交；建议在真实 VPS 上按 DEPLOYMENT.md 部署后观察 RSS 曲线（验证 #1/#2/#4 的泄漏修复）与 CPU per Gbps。
