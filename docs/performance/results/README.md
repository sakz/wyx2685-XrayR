# 原始结果索引

- before.txt：首次同版本业务代码基线；rule 行采用单独 before-rule.txt 的最终正则 benchmark，防止 blocked1 意外先匹配 blocked19。
- after-limiter.txt / after-rule.txt / after-diff.txt：各优化独立复测，5 次。
- recheck-before.txt / recheck-after.txt：预编译后交替版本，dispatcher 与首次连接各 5 次；stdout 中初始化日志可能打断 benchmark 名称行，数值在随后的行。
- reordered-before.txt / reordered-after.txt：逆序用户列表相同 fixture，5 次。早期 recheck-before 中旧测试二进制没有逆序用例，不能作为该项基线；这里的专门复测才是最终数据。
- SUMMARY.md：上述有效数据的中位数。不存在 Linux 生产 CPU/RSS 结果。
- before-dispatcher.txt / after-dispatcher.txt：计时部分受后台编译干扰，已弃用；固定操作量 GC、留存内存/分配数据仍列明实验条件。
- memory-before.txt / memory-after.txt：fixture 的留存 HeapAlloc/HeapObjects，不是全进程 RSS。
- PPROF.txt：同一工具链、1 或 2 个 P 的 CPU/allocs/mutex/block profile 摘要。profile-*-p1/p2.txt 为对应采样运行日志；profile 二进制在本机 /tmp/xrayr-performance-profiles，未加入仓库。
- profile-limiter-before.txt / profile-diff-before.txt：最初用于决策的采样日志；limiter 的原匹配式同时匹配了 1000/10000，最终精确单场景采用 p1/p2 文件。
- baseline-tests.txt / race-final.txt / vet-changed.txt：新增行为基线、最终定向 race、修改相关包 vet。空 vet 日志对应退出码 0。
- all-local-tests.txt / all-local-race.txt / vet.txt：全包检查的既有失败；跳过外部集成的命令见 REPRODUCE.md。
- staticcheck.txt / staticcheck-baseline-export.txt：修改后 18 项、独立导出的原始源码 19 项，差异为旧 diff 死赋值消失；staticcheck-baseline.txt 是失败的 overlay 静态分析尝试，不作为基线。
- linux-build.txt：交叉构建无输出、退出码 0；产物 /tmp/xrayr-performance-profiles/XrayR-linux-amd64，未启动 Linux 服务。
- escape-limiter.txt：Go 1.23.1 -m=2 全量输出；miss 对象仍需逃逸，本次降低分配执行频率。
