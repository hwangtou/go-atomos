# 分布式生命周期梳理与优化计划

> 状态：阶段 0 已实施；阶段 1 及以后为提案
> 日期：2026-10-27
> 范围：Atom 的分布式身份、Spawn、Halt、Drain、超时重试与故障接管
> 关联设计：[Drain](DESIGN_DRAIN_GRADUAL.md)、[Retry/Outbox](DESIGN_RETRY_OUTBOX.md)、[Deadlock](DEADLOCK.md)、[DST](DESIGN_TESTKIT_DST.md)、[v3 API](DESIGN_V3_API.md)

本文是分布式生命周期改造的主文档。它区分三类内容：

- **现状**：当前代码已经实现的行为及其保证边界。
- **阶段 0**：本轮落地的本地生命周期修复。
- **提案**：后续协议和分布式机制，尚未实施，不能当作现有能力依赖。

## 一、结论与目标

go-atomos 已有完整的单机 actor 基础：串行 mailbox、生命周期、实例代际、IDTracker、任务调度、同步死锁检测、类型生成和测试工具。

当前分布式部分更准确的定位是：

> etcd 服务发现 + 用户 Router 显式放置 + gRPC 远程调用。

它还不是带全局 Atom 单例、自动激活、可恢复控制操作和 fencing 的分布式 actor 协议。在 Router 一致、无网络分区、控制操作不做盲目重试时，现有模型可以工作；在超时、响应丢失、节点重启和脑裂下，还需要补充明确协议。

改造目标不是追求字面上的 exactly-once，而是建立以下可验证保证：

1. 同一 Atom incarnation 的 Spawn/Halt 有唯一、可解释的串行结果。
2. 旧实例未停止时，不能仅因超时创建同名替代实例。
3. 远程控制操作不能因进程重启、地址复用或重新路由而作用于错误的 incarnation。
4. 调用方能区分确定未执行、已接受、已完成和结果未知，并能恢复未知操作。
5. 节点失去所有权后不能继续接受需要该所有权的请求；外部副作用通过 fencing 或幂等约束双写。

## 二、当前架构与已有保证

### 2.1 调用与放置

`CosmosLocal.CosmosSpawnAtom` 和 Get 通过 `getGlobalElement` 选择本地或远程 Element。启用集群后，用户实现的 `CosmosMainGlobalRouter` 返回目标节点名；框架不查询 Atom ownership，也不验证不同节点的 Router 决策是否一致。

位置：`cosmos_local.go:184-193`、`cosmos_local.go:338-374`。

### 2.2 节点注册与版本路由

节点使用 etcd lease 注册地址、状态、Element 和 `startup_id`，通过 snapshot + watch 维护对端视图。同名节点的版本锁使用 `ModRevision` CAS，支持热升级和 Draining。

已有价值：

- lease 过期最终删除注册；
- watch 断线后通过 snapshot 对账，避免丢失 DELETE 后留下 phantom node；
- 同地址但 `startup_id` 改变时，对端替换旧连接和版本对象；
- 已解析的远程 Atom ID 固定到连接，避免 current 切换后存量调用误发到另一版本。

位置：`cosmos_etcd.go`、`cosmos_etcd_version_lock.go`、`cosmos_remote.go:119-159`、`base_remote.go:23-127`。

边界：etcd 登记的是节点和 Element，不登记 Atom 所有权。注册健康也不作为 RPC 准入条件。

### 2.3 本地实例代际

每次本地创建会分配进程内单调递增的 `instance_id`。远程消息、State 和 Kill 会精确校验它；同进程 halt + respawn 后，旧 ID 不会隐式作用于新实例。IDTracker 也按 instance 分 cell，旧 tracker 的 Release 不会误减新实例。

位置：`cosmos_process.go:219-227`、`element_local.go:880-917`、`atomos_id_tracker.go`。

边界：实例号在进程重启后重新分配，不能独立承担跨进程 fencing。

### 2.4 Mailbox 与停止

首个 Kill 被 mailbox 取出后，框架关闭新邮件准入并失败化排队邮件，清理异步回调和任务，执行用户 Halt 与可选持久化。普通 Kill 不等待停止；`ArgBaseAtomosWaitKilled` 等待停止结果。

位置：`mail.go:533-561`、`base_atomos.go:829-982`、`atom_local.go:300-354`。

超时只解除调用方等待，不抢占正在执行的用户 handler 或 Halt。对于永久阻塞的用户代码，框架不能无损恢复 goroutine。

## 三、必须维持的核心不变量

### I1. 单一逻辑 incarnation

一个 Atom 名字在任一所有权域内最多有一个可接受新消息的 Running incarnation。旧 incarnation 未终止时，等待超时不能成为创建替代实例的理由。

### I2. 控制操作按 incarnation 串行

同一 incarnation 的 Spawn、Halt、强制接管和 Drain 决策必须有明确顺序。不得并发执行新旧实例的用户 Spawn/Halt 并共享持久化状态。

### I3. 远程身份不可复用

目标远程身份最终必须包含：

```text
logical atom name
atom incarnation（不可复用）
target process startup_id
fencing token
```

进程内 `instance_id` 可以保留用于本地引用计数和兼容，但不能代替跨进程 incarnation。

### I4. 超时不改变事实状态

调用方 deadline 只决定调用方等待多久，不决定目标是否创建、停止或换代。传输失败首先是**结果未知**，不是“确定未执行”。

### I5. Halt 完成是统一屏障

同一停止操作的所有 `WaitKilled` 等待者必须：

- 等待同一次用户 Halt/持久化；
- 观察到相同结果；
- 只在状态进入 Halt、halted hook 和停止通知完成后返回。

普通 Kill 的“已投递”语义不变；ForceHalt 必须是独立、显式、危险的操作。

### I6. 框架锁不跨用户代码

框架锁只保护状态迁移和快照，不得跨用户 Spawn、Halt、持久化或网络调用持有。等待必须可诊断，并尽量支持取消或有界返回。

### I7. 所有权失效必须可执行

lease 失效不仅要改变路由缓存，还必须使旧 owner 的请求准入失效。已建立的 gRPC 连接不能绕过 fencing。

### I8. 不承诺无法实现的 exactly-once

RPC 重试和 WAL 重放天然可能重复。框架可以提供 operation/message 去重窗口和 at-least-once 投递，但任意外部副作用仍需业务幂等或资源级 fencing。

## 四、问题清单与优先级

以下“故障后果”来自代码路径分析；除已有测试覆盖项外，不代表每个交错都已运行复现。

| ID | 级别 | 现状与位置 | 故障后果 | 状态 |
|---|---|---|---|---|
| DL-01 | P0 | Stopping 等 10 秒后直接创建新实例；`element_local.go:632-644` | 新旧 Halt/Spawn 和持久化重叠，破坏同名单例 | ✅ 阶段 0 已修复 |
| DL-02 | P0 | 无 Atom ownership/fencing；`cosmos_local.go:338-374` | 路由不一致或分区时同名多活 | 提案 |
| DL-03 | P0 | keepalive 重注册失败后 gRPC 不自动停止；`cosmos_etcd.go:313-343` | 失租节点仍可接收控制与业务写请求 | 提案 |
| DL-04 | P0 | Spawn/Kill 无 operation ID、去重、结果查询；`atomos_remote_service.go:179-215,470-504` | 响应丢失后无法安全恢复或重试 | 提案 |
| DL-05 | P0 | `instance_id` 跨进程可复用，请求未校验目标 startup/incarnation | stale Kill 可能命中重启后的替代实例 | 提案 |
| DL-06 | P0 | 后排 Kill 提前回复，主 Kill 在 `setHalted` 前回复；`base_atomos.go:829-982` | WaitKilled 不是统一、完整停止屏障 | ✅ 阶段 0 已修复 |
| DL-07 | P1 | 同名 Spawn 锁跨用户 Spawn；`element_local.go:573-580,679-684` | 同名重入或跨名字成环可永久死锁 | 提案 |
| DL-08 | P1 | Halt 持 task stopLock；`base_atomos.go:843-844,974` | Halt/取消回调内调用 Task API 自死锁 | 提案 |
| DL-09 | P1 | 新实例在用户 Spawn 成功前发布；启动失败未走完整停止清理 | waiter/回调/任务不能及时终结 | 提案 |
| DL-10 | P1 | Drain 检查在锁外，Get 自动 Spawn 无检查；`element_local.go:274-295,344-357` | Drain 后仍可能创建 Atom，存量统计失真 | 提案 |
| DL-11 | P1 | `WithRebind` 对连接失败直接重新解析并重发；`id_helper.go:31-60` | Kill/非幂等消息可能重复或作用于替代实例 | 提案 |
| DL-12 | P1 | 远程 Spawn 丢弃 args，未传 CosmosArgs；`cosmos_remote.go:453-462`、`element_remote.go:201-224` | 本地与远程生命周期参数语义不一致 | 提案 |
| DL-13 | P1 | GetIDState 将 Atom 解析错误压成 Halt；`atomos_remote_service.go:100-119` | 不可达、stale、不存在等状态无法用于恢复 | 提案 |
| DL-14 | P1 | 旧实例清理可删除新实例共享的 name 节点；`element_local.go:795-820` | 后续 Element 停止可能漏掉新实例 | 提案 |
| DL-15 | P2 | 远端异步发送失败只清 pending，不以错误终结回调 | 回调语义与 outbox 设计不一致 | 提案 |
| DL-16 | P2 | Draining 状态更新采用非阻塞发送；`cosmos_process.go:794-802` | 其他节点可能继续按 Started 路由 | 提案 |

## 五、阶段 0：先修本地生命周期

阶段 0 不修改公开 API 和 wire protocol，先建立后续分布式协议依赖的两个基础保证。

### 5.1 Stopping 不允许超时重生

Spawn 遇到 Stopping 时：

- 旧实例正常停止：继续创建新 incarnation；
- 等待超时：返回 `ErrAtomosIsStopping`，调用方可稍后重试；
- 禁止覆盖当前容器条目，禁止启动替代实例。

这会把“旧实例卡死”暴露为明确状态，而不是用双活掩盖问题。强制接管留给后续带 fencing 的独立协议。

### 5.2 统一 Kill 完成结果

一次 mailbox 停止只执行一次用户 Halt。主 Kill 和已经排队的重复 Kill 共享结果，并在完整 `setHalted` 后统一回复。普通 Kill 仍然投递即返回。

验收重点：Halt 被 gate 阻塞时，任何 WaitKilled 都不能提前成功；Halt 返回错误时，不能出现一个等待者收到错误、另一个收到 nil。

## 六、目标分布式协议（提案）

### 6.1 控制面与数据面分离

- **控制面**：etcd CAS 决定 ownership、生成 incarnation/fence、记录控制操作状态和结果。
- **数据面**：gRPC 直接传输消息，不为每条消息访问 etcd。
- **Router**：路由提示或缓存，不再是唯一性的正确性来源。

etcd 事务只完成短期状态决策，不能用分布式锁包住用户 Spawn/Halt。

### 6.2 逻辑状态机

```text
Absent → Spawning → Running → Stopping → Halted
              └────────→ Failed / Unknown
```

约束：

- 禁止 `Stopping --timeout--> Spawning`；
- Halt 与 Spawn 并发时，在同一 incarnation 的控制队列中排序；
- Spawning 阶段收到 Halt，可记录 StopRequested，在 Spawn 返回后停止，或通过明确的协作取消终止；
- 替代实例只能在旧实例终止后创建，或由带更高 fence 的显式接管协议创建。

### 6.3 Spawn 流程

1. 调用方为逻辑操作生成稳定 `operation_id`；跨进程重试必须保留它。
2. 通过 etcd CAS 获取 ownership，写入 owner startup、Atom incarnation、fence、operation 和 Spawning 状态。
3. 同一 operation 已存在时返回原操作状态/结果；不同 operation 遇到 Running 实例时维持 AlreadyExists。
4. owner 在锁外执行本地构造和用户 Spawn；Ready 前的消息只能有界等待或返回 Spawning，不能提前执行。
5. 成功后发布 Running；失败后完整清理 waiter、任务和回调，并记录 operation 结果。
6. 客户端超时后通过只读 `LookupAtom` / `GetOperationStatus` 恢复，或以相同 operation ID 重试。

相同 operation 的幂等返回与不同 operation 的重复 Spawn 冲突必须分开处理。

### 6.4 Halt 流程

请求携带：

```text
atom name + incarnation + target startup_id + fence + operation_id
```

处理顺序：

1. 精确校验目标 incarnation，禁止重新解析为同名替代实例。
2. 单次迁移到 Stopping；重复 Halt 共享完成通知。
3. 原子关闭消息准入，失败化排队邮件，快照并取消任务。
4. 释放框架锁，执行用户 Halt 与持久化。
5. 完成清理并进入 Halted。
6. 记录 operation 最终结果，以同一结果唤醒所有等待者。

Halt 超时只返回 StillStopping/Unknown，不允许调用方据此直接创建替代实例。

### 6.5 所有权记录与结果保留

可使用两类记录：

- lease 绑定的 ownership：owner startup、incarnation、fence、状态；多个 Atom 可共享进程 lease。
- 独立保留的 operation/tombstone：用于去重和结果查询，按重试 horizon 设置 TTL 或压实。

ownership 随 lease 消失不应同时抹掉所有 operation 结果。去重只承诺保留窗口；调用方若要求跨自身崩溃重试，也应持久化 operation ID。

### 6.6 失租与强制接管

推荐需要单写保证的部署采用 fail-closed：

- lease/watch 不健康达到判定条件后，旧 owner 拒绝新的所有权相关请求；
- RPC 校验目标 startup、incarnation 和 fence，不能只依赖关闭连接；
- 新 owner 使用更高 fence；
- 外部数据库或资源拒绝低 fence 写入。

已实现可选 `FencedAtomAutoData`：owned ID 带 incarnation/fence 时，Halt
优先调用 `SetAtomDataAtFence`；实现方必须原子拒绝不大于已存 fence 的写入。
未实现该扩展的现有 AutoData 继续走 `SetAtomData`，Legacy 行为不变。

 lease/fencing 不能撤销已经开始的用户 goroutine。若外部副作用不能 fencing，框架只能收窄脑裂窗口，不能宣称物理上绝对单活。无法承受该边界的业务应禁止自动接管，先人工或外部系统确认旧 owner 已失效。

### 6.7 per-Atom 与 shard ownership

- 少量全局单例、需要灵活激活：可从 per-Atom ownership 开始，协议直观，但控制面操作较多。
- 大量 Atom、稳定哈希放置：采用 shard ownership，节点 lease 覆盖多个 shard，减少 etcd 写入；每个 Atom 仍有独立 incarnation。

两种模式共享相同的 Spawn/Halt、operation 和 fencing 语义，不能退回到“多份 Router 配置碰巧一致”。

已实现 `LifecycleModePerShardStrict`：`SetLifecycleShardCount` 配置稳定 FNV-1a
映射（默认 64），shard record/owner 通过同一 CAS 获取或接管并绑定 process
lease。Atom fence 等于 shard fence，同节点同 lease 的后续 Atom 激活不提升
fence；每个 Atom 仍有独立 incarnation、operation 和 owner lookup。Atom 与
shard owner watch 共用本地快照，普通消息不读取 etcd。跨节点接管同时使该
shard 的旧 Atom 准入和旧 operation 写回失败。这里不宣称高规模性能优化完成。

## 七、错误与重试契约

控制操作需要区分：

| 结果 | 含义 | 调用方动作 |
|---|---|---|
| NotExecuted | 服务端明确未接受，未产生副作用 | 可安全重试 |
| Accepted | 已接受，未完成 | 等待或查询 operation |
| Completed | 已有确定成功/业务失败结果 | 使用结果，不重复执行 |
| StillRunning | Spawning/Stopping 尚未结束 | 有界等待后查询 |
| Unknown | 传输失败或恢复信息不足 | 以原 operation 对账，不能假定未执行 |

原则：

- 在 operation/message 去重之前，不对 Kill 和非幂等消息做透明传输重试。
- `WithRebind` 只能用于明确的 stale 引用等安全场景；连接失败不等于安全重试条件。
- 普通异步消息采用 at-least-once 时，message ID 必须在所有重试和 WAL 重放中保持不变。当前通过显式 `ArgBaseAtomosRetryable(messageID)` 启用；首次与重试共享 async ID，成功入队后继续等待业务回复，最终失败才终结回调。
- Lookup/State 必须只读且保留错误细节，不能把 Unknown、Stale、NotFound 全部压成 Halt。

## 八、分阶段实施路线

| 阶段 | 内容 | 完成标准 |
|---|---|---|
| 0 ✅ | Stopping 禁止超时重生；统一 WaitKilled 屏障；主文档与回归测试 | 本地不变量成立，公开 API/wire 不变 |
| 1 ✅ | proto 追加 operation/startup/incarnation/fence；只读 Lookup/OperationStatus；远程 Spawn 参数对齐 | 能识别并对账未知操作，旧错误码不变 |
| 2 ✅ | per-Atom ownership；RPC fencing；失租 fail-closed | 跨节点并发 Spawn 单活，stale Kill 跨重启被拒绝 |
| 3 ✅（受控生命周期） | 同 operation 恢复、结果保留和显式接管；普通异步透明重试/WAL 不在此范围 | 响应丢失后可按原 operation 对账，重复控制操作不重复执行 |
| 4 ✅（per-Atom/per-Shard 与 Drain；高规模优化除外） | Drain 与 ownership 统一；稳定 shard ownership 和重平衡 | Drain 无准入绕过，重平衡有明确 CAS 屏障 |
| 5 | DST、真实故障注入、混部兼容和性能测试 | 关键交错可重放，兼容矩阵和容量边界明确 |

每阶段独立可回滚，不把本地状态机、wire、ownership 和自动重试一次性混入同一改动。

## 九、验证矩阵

### 9.1 本地确定性交错

- Spawning/Running/Stopping 下并发 Get、Spawn、Kill；
- 两个及多个 WaitKilled，Halt 成功、错误和 panic；
- Spawn 失败时 waiter、任务、异步回调全部终结；
- Spawn/Halt 内重入与 A/B 环，不得持框架锁永久等待；
- Drain 在 Spawn 各阶段切入，成功实例必须被存量统计覆盖；
- 旧实例停止和新实例创建不得重叠清理共享容器资源。

### 9.2 分布式故障

- 跨节点同名并发 Spawn，只允许一个 owner；
- 对端已创建/停止但响应丢失，客户端以相同 operation 恢复；
- A 处理中 lease 过期，B 接管，A 回写被 fence 拒绝；
- 节点同地址重启，旧 instance/startup/incarnation 的 Kill 必须失败；
- 网络分区、watch 断流、注册失败但 gRPC 仍可到达；
- Stopping 节点失联，恢复流程不得默认当作已 Halt；
- Drain 广播延迟/失败期间的新 Spawn 和自动激活。

### 9.3 测试方法

优先使用 channel/gate 固定关键顺序，避免用 sleep 猜测竞态。按 [DST 设计](DESIGN_TESTKIT_DST.md) 增加虚拟时钟、etcd 和 gRPC 接缝后，用固定 seed 重放响应丢失、lease/CAS 和控制操作交错。

标准门禁包括根包与子包测试、race、integration、vet/build 和 Windows 交叉编译；协议改造还需真实旧版本与新版本混部测试。

## 十、兼容与实施纪律

- `atomos.proto` 只追加字段，既有字段和错误码数值冻结。
- 新语义先以追加 RPC/可选能力引入，不在同一版本偷偷改变旧字段含义。
- 旧节点不理解 operation/fence 时，必须降级为“不提供新保证”，不能伪报安全。
- 不在没有 operation 身份和幂等契约前上线自动重试或自动接管。
- 每个阶段更新本文状态、关联文档和验收结果，避免设计描述领先于代码。
