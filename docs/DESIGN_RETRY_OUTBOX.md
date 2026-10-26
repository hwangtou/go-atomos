# 设计方案:异步投递重试与 WAL Outbox(进程重启不丢)

> 状态:设计提案(基于代码现状分析,未实施)
> 日期:2026-10-24
> 关联文件:`base_remote.go` / `atomos_remote_service.go` / `element_local.go` / `base_atomos.go` / `cosmos_etcd.go` / `mail.go` / `atomos.proto`
> 关联文档:`DESIGN_V3_API.md` §8(协议演进)、`DESIGN_DRAIN_GRADUAL.md`(逻辑寻址,见开放问题)、`DEADLOCK.md`
> 生命周期与恢复总约束:[`DESIGN_DISTRIBUTED_LIFECYCLE.md`](DESIGN_DISTRIBUTED_LIFECYCLE.md)。传输超时首先是结果未知；通用重发必须以业务幂等或 operation/message 去重为前提，Kill 等控制操作不能直接套用普通异步消息重试。

---

## 一、目标与范围

解决两个投递可靠性场景,**合起来即经典 durable outbox 模式**:

| 场景 | 本质 | 对应机制 |
|---|---|---|
| gRPC 失效、投递失败 | 临时失败,发送方持续重投 | 内存重试器:错误分类 + 指数退避 |
| 发送进程重启 | 待发邮件寿命必须长于进程 | WAL outbox:先落盘,成功再删,重启重放 |

**明确不在本期范围**(讨论中已裁剪):

- handler 执行失败/panic 的消费侧重试(重投邮箱、DLQ、let-it-crash 取舍)——另案;
- 接收端 durable 收件箱(补"收到与处理之间崩溃"的窗口)——Phase 3 之外,默认不做;
- 同步消息重试——调用方语义上就是自己再调一次,框架不透明重试;
- 回复(replay)方向的可靠性——见 §三 语义边界,架构上不可达。

## 二、现状事实(设计依据,基于 v3 分支 0356e02)

| # | 事实 | 位置 | 对设计的约束 |
|---|---|---|---|
| 1 | gRPC `AsyncMessagingByName` 返回成功 = 邮件已入对端 mailbox,不等于 handler 已执行 | `atomos_remote_service.go` `asyncMessagingByName` | ack 边界是"到对端邮箱";接收方收到后崩溃的窗口 outbox 补不了 |
| 2 | 目标 atom 已死时接收端拒绝、**不隐式复活**,返回 `ErrCodeAtomInstanceMismatch` | `element_local.go` `getAtomFromRemote` | 盲目重发永远失败;重试器必须自动 rebind(复用 `WithRebind`) |
| 3 | 回复投给重启后的旧发送进程,被 `startupID` 世代检查拒绝 | `atomos_remote_service.go` `AsyncOnMessageCallback` | 属永久失败,立即终结,不重试 |
| 4 | 节点经 etcd lease 注册,续约/重注册自愈 | `cosmos_etcd.go` keepalive/watch | "目标节点暂时不在路由表"是可重试错误,节点回来自愈 |
| 5 | 回调是发送方内存闭包(`asyncCallbackMap`),不可序列化 | `base_atomos.go` `asyncSet` | 回复方向做不到 durable(见 §三) |
| 6 | 节点注册信息带 `startup_id`,但 Atom `IDInfo` 只有进程内 `instance_id`,目标 RPC 也未校验目标进程代际 | `atomos.proto` / `base_remote.go` | WAL 只存现有 IDInfo 不能保证重放不投错重启后的进程;需先补目标 startup/incarnation/fence |
| 7 | 发送超时(DeadlineExceeded)时邮件可能已投成功,只是回复丢了 | gRPC 语义 | at-least-once ⇒ **handler 幂等是使用契约**;wire 预留全局 message_id(尾部追加,冻结规则允许) |
| 8 | **现存缺陷**:`BaseRemote.PushAsyncMessage` gRPC 失败时,已注册的回调不终结不撤销,悬挂到停机清账才释放 | `base_remote.go` `PushAsyncMessage`(`asyncSet` 之后 `er != nil` 直接 return) | Phase 0 必修;重试器的"永久失败路径"复用同一动作 |
| 9 | outbox 若写 etcd,每封邮件一次 Raft 共识 | etcd 定位:集群元数据 | WAL 落本地盘;整机盘损不在承诺范围(那是副本/外部 MQ 的话题) |

## 三、语义边界(诚实承诺)

| 方向 | 语义 | 原因 |
|---|---|---|
| 请求(发送方 → 对端 mailbox) | at-least-once,可能重复 | 超时歧义 + 重放;消费端幂等是使用契约 |
| 回复(对端 → 发送方回调) | best-effort,发送方重启即在途回调全丢 | 回调是内存闭包,物理上不可恢复;需要可靠回复的业务应反向发 durable 请求 |
| 接收方收到后、handler 执行前崩溃 | 丢失 | 需要 durable 收件箱才能补,本期不做 |

一句话:**durable 请求 + best-effort 回复 + 消费端幂等**。这是 Erlang/OTP 同款取舍。若业务要求完整 MQ 语义,正确演进是给 outbox 加"转投外部 MQ"的桥,而非把框架长成 MQ。

## 四、错误分类(重试器核心决策表)

| 类别 | 典型错误 | 动作 |
|---|---|---|
| 确定未执行,可重试 | 首次发送前已发现无连接、服务端明确拒绝且保证无副作用 | 退避重投,**同一目标串行保序** |
| 结果未知 | 请求可能已发出后的 gRPC Unavailable、DeadlineExceeded、响应丢失 | 普通异步消息只能按 at-least-once 使用同一 message_id 重投并要求业务幂等;Spawn/Kill 先按 operation 查询,不得直接 rebind 重发 |
| 可重试但要 rebind | `ErrCodeAtomInstanceMismatch`(atom 被别处重新 spawn,世代前移) | 仅用于允许迁移到新 incarnation 的消息;重新解析 ID 拿新世代 → 重投,控制操作不得作用于替代实例 |
| 永久失败 | startupID 不匹配、消息名不存在、参数 unmarshal 失败、重试耗尽 | 立即以错误终结回调 + 写死信日志;atom 死了没人重建属于此类——框架不替业务决定重建有状态 actor |

分类函数落在 `atomos_grpc_status.go` 旁边(复用 `grpcErrorToError` 的反映射),作为独立纯函数输出分类结果,Phase 0 抽出。没有服务端明确未执行证明时,传输错误一律先按结果未知处理。

## 五、分阶段实施

| 阶段 | 内容 | 覆盖场景 | 改动面/代价 |
|---|---|---|---|
| Phase 0 | 修 §二#8 的洞:发送失败立即终结回调(镜像 `asyncTerminateSync` 语义);抽错误分类函数 | — | 小;永久失败路径的地基 |
| Phase 1 | 进程级内存重试器:可重试错误退避重投、InstanceMismatch 自动 rebind、耗尽/永久失败 → 错误终结 + 死信日志 | gRPC 失效、节点短暂下线 | 中;纯进程内,不碰盘 |
| Phase 2 | WAL outbox:`ArgBaseAtomosDurable()` 标记的异步邮件先写 WAL 再投,成功删/标,启动重放;接 Phase 1 重试器 | 发送进程重启 | 大;WAL、重放、幂等文档 |
| Phase 3(可选,默认不做) | 接收端 message_id 去重窗口、死信 element、同步方向 opt-in 重试 | 收窄重复率 | 去重窗口本身是内存态,重启即失,做不出严格 once |

### Phase 0:终结悬挂回调

`PushAsyncMessage` 的 `er != nil` / `rsp.Error != nil` 两个失败出口,在返回错误前 `asyncPop(asyncID)` 取回回调并同步以错误终结(带 recover,参照 `asyncTerminateSync`)。注意区分:`ErrCodeAtomosIsNotRunning` 类本地拒绝发生在 `asyncSet` 之前,不受影响;`asyncID == 0` 的停机清账路径维持现状。

### Phase 1:进程级重试器

- 位置:`cosmosProcess` 级单例 retry manager(不挂在任何 atom 上,发送方 atom 死亡不中断重试)。
- 队列:按目标节点分组的串行队列(保序),指数退避(初始 500ms、上限 30s、可配),复用 `time.AfterFunc` 思路(参照 `atomosTaskManager.AddAfter`)。
- 重投:重新走 `BaseRemote.PushAsyncMessage` 全流程(重新 `asyncSet`?否——见开放问题 3)。
- 终结:永久失败/耗尽 → `asyncPop` + 错误终结;`coreError` 死信日志 + 指标 `atomos_retry_dead_letter_total`。
- 进程退出:retry manager 随进程消亡,未竟重试丢失(Phase 2 解决)。
- 测试:`atomostest.RunCluster` 双节点夹具 + gRPC 拦截器注入 Unavailable;断言最终投递、退避单调、永久失败即终结。

### Phase 2:WAL outbox

- 入口:变参标记 `ArgBaseAtomosDurable()`(框架 args 惯例),仅对异步消息生效;默认关闭,零开销。
- 记录:`{message_id, target IDInfo, message name, anypb args, attempts, next_retry_at, created_at}`。载荷本就是 proto,序列化免费。message_id = 节点 boot uuid + 单调 seq,全局唯一。
- 写入路径:`asyncSet` 注册回调 → **先 fsync 写 WAL** → 再发 gRPC;成功 → 删除(或标记 done + 惰性压实);失败 → 交 Phase 1 重试器,重试状态回写 WAL。
- 重放:进程启动、etcd 加入后重放未完成记录,经错误分类后走重试器。重放期间新邮件照常入队。
- fsync 策略:默认逐条 fsync(durable 优先),预留 group commit 调优项。
- 测试:杀进程重启重放(参照 chaos_test.go 的故障注入风格)、WAL 损坏尾部截断、重复投递(断言 at-least-once 下 handler 收到两次)。

### Phase 3(可选)

见上表。启动条件:有业务实测重复率不可接受,或审计要求 message_id 端到端可追。

## 六、协议演进(与 DESIGN_V3_API.md §8 的关系)

- `CosmosRemoteAsyncMessagingByNameReq` 尾部追加 `message_id`(uint64 或 string),Phase 2 引入;v2/旧 v3 节点按 proto3 未知字段跳过,兼容性同 §10.2 `wait_chain` 先例。
- 错误码:重试耗尽建议复用现有超时/失败码 + `RetryExhausted` 文案;确需新增码则 append-only(如 `ErrCodeCosmosRemoteRetryExhausted`),照 §10.2 流程评估。

## 七、开放问题(实施前需拍板)

1. **WAL 与 drain/逻辑寻址的交互**(关联 `DESIGN_DRAIN_GRADUAL.md`):drain 期间目标节点地址变化,重放/rebind 的解析路径是否走改造后的逻辑寻址?建议 drain 落地前 Phase 2 不排期,或 Phase 2 先只支持固定地址投递。
2. **退避与重投的回调归属**:重投时原回调已注册在 `asyncCallbackMap`;若重试耗尽,终结的 error 需携带最终失败原因与尝试次数,API 形态(新增 error code vs 既有码加字段)待定。
3. **重投是否重新注册回调**:不复用原 asyncID 而重新 `asyncSet` 可避开停机清账竞态,但回调闭包需在 retry manager 持有——闭包引用的用户对象生命周期随之延长,需在文档明示。
4. **同一目标保序 vs 全局吞吐**:按目标串行在目标长眠时占内存(邮件驻留);上限(每目标积压阈值)超限策略:降级为拒绝新 durable 发送并报错。
