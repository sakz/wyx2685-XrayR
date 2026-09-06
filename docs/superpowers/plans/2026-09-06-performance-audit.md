# XrayR Performance Implementation Plan

> **For agentic workers:** Use executing-plans to implement this plan task-by-task, inline in the current task.

**Goal:** 在不改变认证、限速、统计、同步行为的前提下，测量并减少 XrayR 自身开销。

**Architecture:** 保持 Xray-core 数据转发、定时调度和 Panel 接口不变。先记录实际调用链，再以同一 Go 1.23.1 工具链测量候选热点；仅保留有明显收益且行为等价的修改。

**Tech Stack:** Go 1.23.1, Xray-core e80ca67feed9, sync.Map, testing/pprof。

## 执行与验收

- [x] 阅读启动、全部 Panel adapter、controller、dispatcher、limiter、rule、证书、构建入口；记录不适用的 core。
- [x] 新增 `common/limiter/limiter_test.go`：重复 TCP/UDP、首次连接、限速开关、在线周期轮换、拒绝分支、并发首次连接；benchmark 100/1000/10000/50000 用户。
- [x] 新增 `service/controller/userdiff_test.go`：对照原算法验证全字段比较、重复用户、增删改和输入不变；benchmark 不变/部分变更/全部变更。
- [x] 新增 `common/rule/rule_test.go`：正则匹配、命中去重、首规则优先、非法 UID、提取结果；benchmark 命中/不命中。
- [x] 修改业务代码前运行 `GOTOOLCHAIN=go1.23.1 GOMAXPROCS=1 go test ./common/limiter ./common/rule ./service/controller -run '^$' -bench=. -benchmem -count=5`，保存原始数据。
- [x] 对重复 TCP 连接和大型 diff 采集 CPU/allocs profile。若证据支持，limiter 使用 Load 命中后复用、miss 仍 LoadOrStore；diff 使用一个预分配状态 map，保留重复输入语义；rule 使用 MatchString 并复用已存在结果 set。
- [x] 每个候选修改后重复同样 benchmark；没有明显收益就撤回该候选。
- [x] 执行本地回归、race、vet、逃逸分析及 Linux 交叉构建。现有需要真实 Panel、ACME 或信号退出的测试单独列为外部集成测试，不伪报通过。
- [x] `docs/performance/REPORT.md` 记录链路图、生命周期、共享状态/锁、前后数据和生产 Profile A/B/C 测量步骤。结果文件存 `docs/performance/results/`。

## 约束

不更换 map 并发模型，不引入池、缓存或 scheduler，不修改 GOGC/GOMEMLIMIT，不改变上游 relay，不提交或推送。发现的原有统计/生命周期正确性风险单列，不混入无基线的大修。

验证收尾：相关回归/race/vet 和 Linux 构建完成；外部集成未执行，全包原有失败和 staticcheck 诊断明确记录，不将步骤执行完毕等同于全仓库通过。
