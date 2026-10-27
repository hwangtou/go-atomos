# Task 延迟任务定时器调度评估

状态：暂不实施（2026-10-28）。保留基于 `Clock.AfterFunc` 的当前实现；只有 profiling
和 benchmark 证明海量并存 timer 已成为主要瓶颈时，才重新评估共享定时器队列。

## 背景

`Task.AddAfter` 当前在 `atomos_task_manager.go` 中通过
`at.atomos.process.getClock().AfterFunc` 创建定时器。到期回调在 manager 锁内检查任务
状态，然后将邮件投入 mailbox；`Cancel` 通过 `Timer.Stop` 取消尚未到期的任务。
带 delay 的 Task queue 在 `atomos_task_manager_queue.go` 中使用同样的机制。

曾评估改为“一个 goroutine 循环处理最小堆定时器队列”，希望减少 timer 开销。
当前结论是收益不足以抵消正确性、并发和维护成本。

## 暂不实施的原因

- `time.AfterFunc` 在等待期间不会为每个 timer 独占一个 goroutine；Go runtime
  已统一管理定时器，到期后才启动 goroutine 执行回调。不存在 N 个延迟任务就有
  N 个常驻 goroutine 的问题。
- 自研调度器仍需保留现有的 task、mail、manager 锁和取消逻辑，只能减少一部分
  runtime timer 开销。任务和邮件的分配、锁竞争可能才是更值得先优化的部分。
- 每个 `BaseAtomos` 都有独立的 task manager。若为每个 manager 启动一个调度
  goroutine，大量没有延迟任务的 Atomos 也要承担常驻 goroutine，可能反而增加开销。
- 单个 goroutine 串行处理所有到期回调会引入队头阻塞，降低定时精度，并改变当前
  到期任务可以并发投递的行为。
- 新实现必须重新保证 `Cancel` 与到期竞争、停机清理、任务状态转换以及框架
  `Clock`/虚拟时钟 DST 的语义，正确性成本不低。

## 重新评估的触发条件

仅在以下条件同时满足时启动实现评估：

1. 同一进程中长期存在数万至数十万以上延迟任务，或创建/取消频率极高。
2. pprof、内存 profile 和 benchmark 已确认 timer 是主要开销，而非 task/mail
   分配、闭包捕获或 manager 锁竞争。
3. 已有对照 benchmark，至少覆盖并存 timer 数量、创建/取消吞吐、到期延迟、
   内存占用和 goroutine 数量。

## 若实施，推荐架构

- 每个 `CosmosProcess` 使用一个共享 scheduler，不按 Atomos 或 task manager 创建
  goroutine。
- 使用最小堆管理 deadline，只维护一个指向最近到期任务的 timer。
- 取消采用标记或 generation，堆节点惰性删除，避免每次取消都执行堆删除。
- scheduler 只负责快速投递，不在调度循环中执行用户任务或可能阻塞的回调。
- 同时覆盖 `AddAfter` 和 delayed Task queue，避免只优化一条路径而收益有限。
- 不在持有 manager 锁时调用 scheduler；明确 scheduler 锁与 manager 锁的顺序，
  防止到期、取消和停机之间死锁。
- 保留 `Clock` 抽象，并补充真实时钟与虚拟时钟下的取消竞争、同刻到期、停机和
  精确到期边界测试。

若目标实际是大量短周期且允许一定时间误差的任务，应优先比较时间轮；时间轮通常比
“单 goroutine + 最小堆”更适合这种负载。
