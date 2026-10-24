# 阻塞缓解增强（B1 可观测 / B2 看门狗 / B3 分片）

状态：提案（2026-10-24）。补齐 [DEADLOCK.md](DEADLOCK.md) 三层防御之外的
遗留空白；均为新增能力而非缺陷修复，实施另立里程碑。

## 问题定义

DEADLOCK.md 解决的是**同步调用成环**：检测与超时让环上的调用者带着错误
返回。它明确不覆盖的三类空白：

1. **慢 handler 不可见**。mailbox 深度与 handler 执行时长没有任何指标
   暴露，热点/退化只能在调用方超时或 `DeadlockEvent(Timeout*)` 事后倒查。
2. **卡死无进程内解**。Go 不能抢占 goroutine：handler 阻塞在外部资源或
   用户锁上时，kill 只是排队（控制 mail 排在卡住的 handler 后面），
   唯一手段是重启进程（DEADLOCK.md §Runtime handling）。
3. **单 atom 吞吐上限**。一个 atom 一条 mailbox goroutine 串行处理，
   热点 atom 的吞吐上限就是一条 handler 的串行速率（队头阻塞）。

对应三件事：B1 先看见，B2 事后止损，B3 提高上限。

## B1 — mailbox 深度与 handler 时长可观测

最高优先；纯增益、零破坏，且是 B2 的前置。

基建已在：debug server（`/metrics` 手写 Prometheus 文本、`/elements`
清单 + wait graph，见 `debug_server.go`）、`mailBox.getNum()`
（`mail.go`）、`allocMailDebugMap`、wait-graph、`GetIDIdleTime`。
缺的是把 handler 执行路径接上：

- `atomos_mailbox_depth{element,atom}` gauge：`getNum()` 采样；同时输出
  高水位（since last export 的 max），避免瞬时尖峰被采样率抹掉。
- `atomos_handler_duration_seconds{service,method}` histogram：
  `onSyncMessaging` / `onAsyncMessaging` 的 handler 执行段计时。排队
  等待时长单独一个直方图——区分"handler 慢"与"积压深"是排障的关键。
- `/elements` 页面增加按深度排序的 atom 列表与最慢 handler 面。

验收：压测某个 atom，深度随负载升降可见；注入慢 handler 后能在直方图
中指认到 service.method。

## B2 — handler 看门狗：超时告警 → atom 级重建

分两阶段，先观测后处置。

**阶段 1（告警）**：handler 执行超过阈值（默认关闭，
`SetHandlerWatchdog(d)` 开启）→ 触发 `DeadlockEvent` 扩展 kind
（`HandlerStuck`）+ `DumpStacks()`。复用 B1 计时与 `SetDeadlockHook`
管道，零风险。

**阶段 2（重建）**：把"重启进程"缩小为"放弃并替换该 atom 的 mailbox
goroutine"。设计要点：

- **放弃而非终止**：卡死的 goroutine 无法真正结束，只能泄漏。重建 =
  旧 atom 标记 wedged、其 startupID 作废（复用现有 stale-reply 拒绝
    路径），新 mailbox goroutine 以同 ID 语义接替处理后续 mail。泄漏
  必须显式计量（`atomos_wedged_atoms_total`）并在 `/elements` 标注，
  不能静默。
- **kill-queued 语义不变**：DEADLOCK.md 承诺控制 mail 的 10s 超时下限
  与排队语义。重建时旧 mailBox 中未处理 mail 的归属需要定义：倾向
  丢弃并按 `ErrAtomosPushTimeoutHandling` 语义回错。
- **触发面**：只对显式开启看门狗的 element 生效，避免误杀慢业务。

验收：handler 阻塞在永不返回的 chan 上，开启看门狗后该 atom 在阈值后
被重建，后续调用由新实例处理并拿到明确错误而非无限等待；旧 goroutine
泄漏被计数。

## B3 — 热点 atom 路由池/分片

提高单逻辑服务的吞吐上限。设计面最大，单独排期或与大版本窗口对齐。

- element 层配置 pool size（`ElementConfig` 扩展），spawn 语义从
  "唯一 atom 名"扩展为"池内 N 个实例 + 路由策略（哈希/轮转/最空闲）"。
- 关键难点是 **ID 语义**：IDTracker/ID 定位与远端路由（etcd 注册表按
  atom 名）目前假设一名一 atom；池化后调用方拿到逻辑 ID 还是实例 ID、
  跨节点如何定位需要设计。与遗留 scaling 分支的关系：**不复活旧分支**
  （其解法已被行业超越），按当前 ID 体系重新设计。
- 适用边界：无状态转发型服务收益最大；有状态 atom 不适用（分片破坏
  单线程状态语义），文档需明确。

## 优先级与依赖

B1（复用内窥基建，纯增益）→ B2 阶段 1（依赖 B1 计时）→ B2 阶段 2
（语义设计重）→ B3（独立设计面，可与大版本窗口对齐）。

B1 是路线图第 1 步（内窥）的直接延伸；B2/B3 为新增能力。B2 阶段 2
的"重建"场景天然是 DST（DESIGN_TESTKIT_DST.md）的确定性验证用例。
