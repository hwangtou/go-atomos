# 确定性模拟测试（DST）与 I/O 接口化设计

状态：实施中（2026-10-27）。Clock 接口、`atomostest.VirtualClock`、核心生产计时点迁移、内存 etcd 契约实现和 gRPC 故障注入器已落地；etcd 生产调用点迁移、cron、远程 I/O Deadline 与完整内存互连仍待完成。

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

设计：`CosmosProcess` 持有 `clock` 接口（Now/After/AfterFunc/NewTimer/NewTicker），
生产实现为直通标准库；`CosmosRunnable.SetClock(...)` 注入，nil 保持 systemClock。
难点是 `robfig/cron` 内部直读真实时间——需要以 cron 表达式解析 + 自管 ticker
的薄实现替换（保留表达式语义，丢弃其调度器），cron 暂不随本轮迁移。

当前实现：`clock.go` 提供 Clock/Timer/Ticker，`atomostest/clock.go` 提供稳定
同刻顺序、Reset/Stop、Ticker 和嵌套回调；内部测试通过 `internal/testclock`
复用同一实现，避免根包测试与 atomostest 的导入环。`CosmosProcess` 初始化、
async retry 与 drain 使用同一个注入 Clock。

已迁移的生产语义计时点：

- `baseAtomosMail.waitReply` / `waitReplyID` 的 reject/handling 超时；所有 channel
  timer 都在回复或超时退出时 Stop。
- `Task.AddAfter`、queue delay、message-timeout tracer 的 AfterFunc；任务 timer
  改为可 Stop 的框架 `Timer`，取消语义不变。
- drain deadline/ticker，以及 atom spawn 等待旧实例退出、等待 ready 和 stop gap。
- async retry 的首次调度与指数退避。
- etcd registration retry、info publish retry、watch reconnect/snapshot backoff。
- lifecycle operation 总等待与轮询、owner watch reconnect/probe、janitor ticker；
  operation 的开始/结束和保留时间也读取同一 Clock，使虚拟 janitor 能判断过期。
- `TestDST_` 覆盖 mailbox reject/handling、AddAfter、queue delay、drain deadline
  和 retry backoff，均断言未 Advance 不触发，且 Advance 到精确边界才触发。

明确保留真实时间的点：

- cron 仍由 `robfig/cron` 调度；迁移时必须保持表达式、重复任务和取消语义。
- gRPC/etcd 网络请求 context deadline、dial、etcd keepalive 协议周期，以及
  `BaseRemote`/`CosmosRemote` 的远程调用 deadline；它们依赖真实传输，留待传输
  与 etcd 接口化后处理。etcd version-lock 的 Sleep backoff 也尚未迁移。
- process Stop 的 100ms goroutine 收尾等待和 mailbox `waitExit` 的真实轮询，用于
  等待真实 goroutine 退出，不作为业务虚拟时间推进。
- startupID、日志、消息统计与 RPC metrics 的墙钟时间不做机械替换。

### B. etcd 客户端

`CosmosProcess.cluster.etcdClient` 的使用面已经收敛在少数函数
（keepalive、version lock CAS、watch、注册 PUT/GET/DELETE）。定义
`etcdKV` 接口覆盖这些操作，生产实现包 `clientv3`；测试实现为内存 map +
租约到期回调（虚拟时钟驱动）+ watch 订阅者列表，复刻 CAS 语义。
故障注入套件（716b9d9）的场景可以逐个迁移为确定性版本。

当前实现：`internal/etcd` 提供 `Client` / `Txn` / `Op` 窄接缝，覆盖
Get/Put/Delete/Txn/Watch/Grant/KeepAlive/Revoke/Close。根包以
`EtcdClient` / `EtcdTxn` / `EtcdOp` 别名公开该接缝，`atomostest` 保留同名
兼容别名与操作构造函数。请求、响应、Compare 和选项使用 `clientv3` 类型；
Put 与事务 `EtcdOpPut` 显式携带 LeaseID，避免用 unsafe/反射读取
`clientv3.Op` 的私有 lease。生产 adapter 位于 `internal/etcd`，真实启动和
共享契约测试共同复用它。`CosmosRunnable.SetEtcdClient` 可注入 MemoryEtcd；
process 只关闭自己创建的客户端，外部注入的共享后端由调用方管理。

已实现的内存语义：

- revision 从 1 开始全局递增，同一事务/lease 删除共享一个 revision；key
  按字节序返回，Create/ModRevision、Version 与 Lease 元数据随写正确变化。
- Get prefix/range/from-key、limit/count-only/keys-only、正 revision 快照与
  mod/create 过滤；revision<=0 读取当前状态，keys-only 只清除 Value 并保留
  其他元数据。Put/Delete 及事务原子提交；future compaction 返回 ErrFutureRev。
- Txn If 的 ModRevision/CreateRevision/Version/Value 与 =/!=/>/<，Then/Else
  及事务内 Get/Put/Delete；缺失 key 的 Value 比较恒为 false，空 range 的
  Value 比较为 false，其他 target 对一个全零 KV 比较一次；失败预校验不会
  留下部分写入。
- Watch 从指定 revision 含起点回放 PUT/DELETE，支持 prefix/range 和 context
  cancel；compacted 起点返回终止响应，future 起点保持等待并在 revision 到达后
  发送事件。cancel、DisconnectWatches 和 Close 会唤醒阻塞发送并关闭 channel；
  可注入断流并以 WithRev 恢复重放。
- lease TTL 完全由共享 `VirtualClock` 驱动，KeepAlive 立即并按 TTL/3 续约；
  过期和 Revoke 在同一 revision 删除全部绑定 key 并产生 DELETE。
- Get/Put/Delete/Txn/Watch/Grant/KeepAlive/Revoke 支持持续或单次错误注入；
  watch 另支持无终止响应断流和 canceled 终止。
- 同一 contract 在默认测试跑内存实现，在 integration 标签下无 skip 地跑真实
  embedded etcd，覆盖 CAS、缺失 key/空 range 比较、keys-only 元数据、watch
  回放/future revision/compaction、revision 边界、lease 续约/过期/撤销及事务
  lease。阻塞发送时的 cancel/DisconnectWatches/Close 另有内存实现专项测试。

未覆盖语义：嵌套事务、非默认 Sort、PrevKV、IgnoreValue/IgnoreLease、Lease
Compare、watch filters/progress/created/fragment、鉴权与多 endpoint 暂不在窄契约
内；不要把相应选项的静默缺省当作真实 etcd 支持。内存断流采用调用方显式
WithRev 恢复，不模拟 clientv3 的自动重连；watch 故障注入只生成 canceled/compacted
响应，不伪造任意私有 close error。生产调用点迁移及既有 chaos 场景的 DST 化仍待完成。

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
