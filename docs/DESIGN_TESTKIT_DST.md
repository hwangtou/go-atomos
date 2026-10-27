# 确定性模拟测试（DST）与 I/O 接口化设计

状态：实施中（2026-10-27）。Clock 接口、`atomostest.VirtualClock` 和 gRPC 故障注入器已落地；生产调用点迁移与内存 etcd 仍待完成。

## 目标

确定性模拟测试（Deterministic Simulation Testing，FoundationDB/TigerBeetle 模式）：
在单进程内以虚拟时钟驱动多个"节点"，使故障注入与时序完全可复现——同一 seed
重放同一交错。当前测试体系（2026-10-24 现状）已具备：

- 真实双节点 fixture（`atomostest.RunCluster`：embedded etcd + 真实 gRPC）——
  真实但非确定性，时序敏感断言只能用 Eventually 容忍；
- chaos_test（drain+kill 优雅降级）、etcd 故障注入套件（断连/lease 过期/脏数据）。

DST 的缺口是**三个 I/O 接缝**都直连真实实现。

## 三个接缝与改造设计

### A. 时钟（最高收益，先做）

框架热路径的计时点：`baseAtomosMail.waitReply` 的 `time.After`、任务调度器的
延时/定时任务与 cron、etcd keepalive 周期、drain 轮询间隔。

设计：`CosmosProcess` 持有 `clock` 接口（Now/Since/After/NewTimer/NewTicker），
生产实现为直通标准库；`CosmosRunnable.SetClock(...)` 注入。难点是
`robfig/cron` 内部直读真实时间——需要以 cron 表达式解析 + 自管 ticker 的薄实现
替换（保留表达式语义，丢弃其调度器），这是唯一的破坏性子步骤，建议与 D1/D2
同窗口评估。

落地顺序：先 mailbox 超时与任务延时（waitReply/AfterFunc 两类调用点），后
cron。虚拟时钟实现（手动 advance + 唤醒等待者）放 atomostest。

当前实现：`clock.go` 提供 Clock/Timer/Ticker，`atomostest/clock.go` 提供稳定
同刻顺序、Reset/Stop、Ticker 和嵌套回调；`CosmosRunnable.SetClock` 与生产
调用点迁移尚未完成。

### B. etcd 客户端

`CosmosProcess.cluster.etcdClient` 的使用面已经收敛在少数函数
（keepalive、version lock CAS、watch、注册 PUT/GET/DELETE）。定义
`etcdKV` 接口覆盖这些操作，生产实现包 `clientv3`；测试实现为内存 map +
租约到期回调（虚拟时钟驱动）+ watch 订阅者列表，复刻 CAS 语义。
故障注入套件（716b9d9）的场景可以逐个迁移为确定性版本。

### C. gRPC 传输

跨节点调用走 `BaseRemote/ElementRemote` 的 `getCli`。定义传输工厂接缝
（dial/conn），测试实现为进程内内存管道（带可注入延迟/分区/重排）。
"多节点"退化为多个 CosmosProcess + 内存互连，`_cluster_advertise_ip`
经验（loopback 优先）在此不再需要——根本没有 TCP。

当前实现：`atomostest/fault.go` 可通过真实 gRPC client interceptor 注入
drop-request、执行后 drop-response、delay/hold、duplicate/reorder、blackhole
和 heal，并记录事件顺序。生产 dial factory 和全节点内存互连尚未完成。

## 验收样例（设计目标）

以虚拟时钟重放"节点 A 处理中、etcd lease 过期、B 抢占版本锁、A 回写"
的交错，seed 固定则断言时序逐微秒一致；同一用例注入 100ms 分区与不注入
分别断言两种结局。这类断言今天写不出来——真实时间下 lease 过期是睡出来的。

## 冻结决策：G5/G7/G9 与 Messenger 泛型精简

`docs/PROTO_GEN_IMPROVEMENTS.md` 的 G5（Messenger 泛型冗余）、G7（noExport
简陋）、G9（Spawn 签名约定）均为**破坏性**生成 API 变更。v3 已过 rc1：§10
兼容矩阵（wire 字节比对、131 错误码零偏移）验证建立在当前生成物之上，此刻
改动会使验证失效。决策：冻结至下个大版本窗口（与 D1/D2 类型安全改造同批），
在彼处的 CHANGELOG 以迁移条目落地。本决策记录于 DESIGN_V3_API §6.1 待办处
不再重复展开。
