# 弱配置 VPS 部署调优指南

目标：在内存和 CPU 都有限的 VPS 上服务更多用户、压榨带宽、提升单用户速度。
本指南不改代码，配合 `release/config/99-xrayr.conf` 与 `release/config/xrayr.service` 使用。

## 1. 内核层（收益最大的单项）

```bash
cp release/config/99-xrayr.conf /etc/sysctl.d/
sysctl --system
```

要点：

- `bbr + fq`：高延迟高丢包跨国链路的吞吐决定因素，默认 CUBIC 会把带宽压得很低。
- `rmem_max/wmem_max`：按 BDP（带宽×RTT）估算，示例值 32MB 覆盖 200Mbps×1s 以内链路。
- `tcp_notsent_lowat`：限制每连接内核缓冲积压，高并发时显著省内存。
- 文件描述符：`xrayr.service` 已设 `LimitNOFILE=1048576`。

## 2. Go runtime

`GOMEMLIMIT` 设为物理内存的 75~85%，同时放宽 `GOGC`（示例 300）。
GC 只在接近内存上限时高频运转，平时少跑 GC——用富余内存换 CPU 时间，这正是弱 CPU VPS 想要的交易。
修改 `xrayr.service` 中 `Environment=GOMEMLIMIT=...` 后 `systemctl daemon-reload && systemctl restart xrayr`。

## 3. XrayR 配置（config.yml）

| 配置 | 建议 | 说明 |
|---|---|---|
| `ConnectionConfig.BufferSize` | 内存紧张降到 32；追求单连接大带宽升到 128~256 | 每方向 pipe 的积压上限(KiB)，默认 64。低内存+高并发降；高 BDP 提速升 |
| `ControllerConfig.DisableSniffing` | 无域名分流/审计规则时设 true | 每连接省最多 2×100ms 首包等待与拷贝 |
| `ControllerConfig.DisableGetRule` | 不用面板审计规则时设 true | 省每周期规则拉取 |
| `ControllerConfig.UpdatePeriodic` | 大用户量(>2万)从 60 提到 120~300 | 同步风暴减频 |
| `DnsConfigPath` + `DNSType: AsIs` | 配带缓存的低延迟 DNS | Xray DNS 内置缓存，降低新域名连接延迟 |
| `Log.Level` | none（默认已是） | 日志有 CPU 与 IO 成本 |

## 4. 协议选型（订阅分发层面）

- **VLESS + TCP + REALITY**：单层加密、CPU 开销最低，是弱 CPU 首选。
- 避免 WS/gRPC + TLS 除非必须过 CDN——双层 TLS 对弱 CPU 是约 2 倍加解密开销。
- REALITY 见 `release/config/config.yml.example` 中 `EnableREALITY` 一节。

## 5. 快速自检

```bash
sysctl net.ipv4.tcp_congestion_control   # 应为 bbr
cat /proc/$(pidof XrayR)/limits | grep files   # 应为 1048576
journalctl -u xrayr -f                    # 确认启动正常
```
