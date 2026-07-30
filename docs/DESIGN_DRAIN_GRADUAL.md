# 设计方案：有状态 Atom 的灰度 Drain 与 ID 寻址改造

> 状态：设计草案（基于业界调研 + 现状代码分析）
> 日期：2026-07-30
> 关联文件：`cosmos_etcd.go` / `cosmos_remote.go` / `base_remote.go` / `atom_remote.go` / `element_remote.go` / `cosmos_local.go` / `cosmos_process.go` / `atomos.proto`

---

## 目录

- [一、目标](#一目标)
- [二、现状的根本缺陷：ID 寻址模型不支持 drain](#二现状的根本缺陷id-寻址模型不支持-drain)
- [三、业界最佳实践（调研结论）](#三业界最佳实践调研结论)
- [四、改造方案：逻辑寻址 + 连接池 + drain 状态机](#四改造方案逻辑寻址--连接池--drain-状态机)
- [五、分阶段实施](#五分阶段实施)
- [六、向后兼容性](#六向后兼容性)
- [七、风险与边界](#七风险与边界)
- [八、与审计问题的关系](#八与审计问题的关系)
- [九、验证方案](#九验证方案) ← 如何证明改造正确、生产可靠

---

## 一、目标

支持**老版本 drain 式灰度退出**：

1. 新版本节点起来后，接手全部**新流量**（新 spawn 的 atom）
2. 老版本节点收到 drain 指令后，**停止接新流量**，只服务**存量 atom**（进行中的房间对局）
3. 存量 atom 自然结束后，老版本节点**自动优雅退出**
4. 全程**不迁移内存状态**、**不断裂已建立的存量连接**

前提约束：框架里的 atom 多数是有状态的（房间对局），状态在进程内存，**不能靠切流量迁移**。

---

## 二、现状的根本缺陷：ID 寻址模型不支持 drain

### 2.1 当前寻址链路

调用方（如 player atom）持有远程 room atom 的 ID 时，引用链：

```
*AtomRemoteInSourceProcess（调用方持有）
  └─ AtomRemote { remote BaseRemote{cosmos, info}, element *ElementRemote }
```

**所有 gRPC 调用最终走这一行**（`base_remote.go:24`）：
```go
cli := a.cosmos.getCurrentClient()   // 每次现取 CosmosRemote.current 的连接
```

`CosmosRemote` 是按 nodeName 共享的单例，`current` 是单值指针（`refresh()` 设置）。
**AtomRemote 不记住"我的 atom 在哪台机器"**，把寻址权完全交给会变的 `current`。

### 2.2 状态切换时的致命场景

drain 灰度时 current 会切换（V1→V2），此时：
- 持有 V1 上 room#42 的 AtomRemote 调 `getCli()` → 拿到 **V2 的连接**
- 请求发到 V2 → V2 找不到 room#42 → **`ErrAtomNotExists`，存量对局断裂**

这是 drain 的**阻断性前提**：当前模型下，current 一变，所有指向旧版本存量 atom 的 ID 全部失效。

### 2.3 附带的现存 bug（getCli 不可靠）

即使不考虑 drain，`cosmosRemoteVersion` 的连接生命周期也有缺陷：
- `setDisable()`（cosmos_remote.go:417）会 `client.Close()` 但**不置 nil**，后续 `getCli` 拿到已关闭连接
- `dialOnce sync.Once` 是一次性的，Close 后**无法重拨**
- `etcdUpdateVersion` 地址变化时新建 version 对象替换 map，旧对象的连接成孤儿

---

## 三、业界最佳实践（调研结论）

调研了分布式 Actor 框架（Akka Cluster Sharding、Orleans）和 Actor 设计理论，三个共识模式：

### 模式 1：逻辑身份与物理地址解耦
> "An actor's identity is entirely logical, not tied to a physical address." — With Blue Ink

Actor ID（type + entityId）是逻辑标识。调用方持有逻辑引用，运行时负责解析到具体节点。
Akka 的 `ActorRef`、Orleans 的 `GrainReference` 都是逻辑引用。**这是我们框架缺的**。

### 模式 2：hostId 防止过期引用
> "Each host generates a random ID on startup... clients cache (address, hostId). If they differ, the host was restarted, prompting re-query." — With Blue Ink

每个节点启动时生成随机 `hostId`。定位响应返回 `(address, hostId)`。调用方缓存并带上请求。
目标节点比对，不匹配则触发重新定位。**比纯按 address 池化更可靠**（防 address 复用）。

### 模式 3：迁移期消息缓冲（Akka Cluster Sharding）
> "Messages to a migrating shard are buffered until rebalance completes, then redirected." — Petabridge

ShardRegion 本地缓存 shard 位置，rebalance 时缓存失效；迁移期间的请求**缓冲**，完成后重定向。

### 业界印证
没有任何成熟框架走"缓存可变版本对象指针"（我们原讨论的路径 A）这条路。
业界统一走**逻辑身份 + 地址缓存 + 失效校验**。

---

## 四、改造方案：逻辑寻址 + 连接池 + drain 状态机

借鉴业界模式 1+2+3，结合框架现状（已有 `startupID`、`AtomAutoData`）。

### 4.1 核心思想：ID 记住逻辑定位，按定位寻址

AtomRemote 不再每次问 `cosmos.current`，而是：
- **创建时记住逻辑定位** `(address, startupID)` —— 不可变
- **getCli 按定位取连接** —— 从按 address 池化的连接池取，与 current 解耦
- **请求带 startupID 校验** —— 目标节点比对，不匹配则返回"引用过期"，触发调用方重新定位

这样 current 切换**不影响**存量调用方——它们仍按自己的 `(address, startupID)` 寻址到老版本。

### 4.2 复用已有基础设施

| 需要 | 现状 | 复用情况 |
|---|---|---|
| 进程唯一标识（hostId） | `CosmosProcess.startupID = time.Now().UnixNano()` | ✅ 已有，每次启动唯一，只需广播 |
| Atom 持久化恢复 | `AtomAutoData.GetAtomData/SetAtomData` | ✅ 已有（用于 Halt 保存/Spawn 加载） |
| 节点状态广播 | `CosmosNodeVersionInfo{State}` 经 etcd watch | ✅ 已有，只需加 Draining 状态值 |
| 远程 RPC 通道 | `AtomosRemoteService` gRPC | ✅ 已有，只需加 drain 指令 RPC |
| 后台监控 goroutine | `ElementStartRunning.StartRunning()` | ✅ 已有，可用于 drain 存量轮询 |

---

## 五、分阶段实施

### 阶段 0：修复 getCli 现存 bug（前置）

无论是否做 drain，`cosmosRemoteVersion` 的连接管理都该修：
- `setDisable()`：Close 后置 `client = nil`、`avail = false`
- `dialOnce`：改为可重置（Close 后允许重新拨号），或按 `avail` 标志判断是否重拨
- `getCli`：取到 nil client 时若版本仍 enable 则重拨

**改动**：`cosmos_remote.go`（cosmosRemoteVersion 生命周期），不涉及 proto。

### 阶段 1：startupID 广播 + IDInfo 携带定位

**proto 改动**（向后兼容，加字段）：
```protobuf
message CosmosNodeVersionInfo {
  string node = 1;
  string address = 2;
  IDInfo id = 3;
  ClusterNodeState state = 4;
  map<string, IDInfo> elements = 5;
  uint64 startup_id = 6;   // 新增：进程启动标识，每次启动唯一
}
```
> 注意：proto3 加字段天然向后兼容（旧节点不填则为 0）。

**代码改动**：
- `prepareClusterLocalNode`：写入 etcd 时带上 `p.startupID`
- 远程 atom 创建时（`newAtomRemoteInSourceProcess`）：把创建时刻的 `(address, startupID)` 存进 AtomRemote（作为逻辑定位）

### 阶段 2：按 address 池化连接 + getCli 解耦 current

新增连接池（在 `CosmosProcess.cluster` 上）：
```go
type clusterConnPool struct {
    mu    sync.Mutex
    conns map[string]*pooledConn   // key = address
}
type pooledConn struct {
    conn     *grpc.ClientConn
    dialOnce sync.Once
    lastUse  time.Time
}
```
- `getCli(addr)` 按 address 取/建连接（懒拨号，sync.Once 保证只拨一次）
- AtomRemote 的 `getCli` 改为用自己的 `address` 查池，**不走 cosmos.current**
- 连接池定时清理"长期空闲且无 AtomRemote 引用"的连接（LRU 兜底，无需精确引用计数）

**效果**：current 切换完全不影响存量调用方；老版本的连接在其存量 atom 存活期间一直有效。

### 阶段 3：startupID 校验 + 引用过期重定位

- 远程调用请求带上 AtomRemote 的 `startupID`（复用现有 RPC 的 header/字段）
- 目标节点比对 `req.StartupId != process.startupID` → 返回专门的"引用过期"错误（区别于 ErrAtomNotExists）
- 调用方收到"引用过期"→ 回退到 `getGlobalElement` 重新解析 → 拿到新定位

**效果**：address 被复用（同 IP:port 重启新进程）时不会误连；节点崩溃重启后调用方自动重新定位。

### 阶段 4：Draining 状态 + drain 状态机

**proto**：
```protobuf
enum ClusterNodeState {
  ClusterNodeStateInvalid = 0;
  Starting = 1;
  Started = 2;
  Stopping = 3;
  Stopped = 4;
  Draining = 5;   // 新增：停止接新流量，只服务存量
}
```

**路由绕开 Draining**（`refresh()` 改造）：
- 遍历 `c.version`，优先选 `State == Started` 的版本作 current
- 仅当所有版本都 Draining/Stopping 时才 disable
- 这样新版本（Started）自然接手新流量

**drain 指令**（新增 RPC `DrainNode`，或扩展 TryKilling）：
被调用的节点：
1. 把自己 `ClusterNodeState` 改为 Draining，写回 etcd（触发其他节点刷新路由）
2. 启动 drain 监控 goroutine

**存量归零自动退出**（drain watcher）：
- 轮询所有 element 的 `GetActiveAtomsNum()`
- 全部归零 → 调 `ExitApp()` 优雅退出
- 超时兜底（配置 `DrainDeadline`，默认如 1h）→ 强制进入 Stopping

### 阶段 5（可选）：迁移期消息缓冲

借鉴 Akka，AtomRemote 收到"引用过期"时不立即报错给业务，而是：
- 短暂缓冲请求（如 200ms）
- 重新定位成功后重发
- 定位失败再上抛错误

降低迁移窗口对业务的瞬时冲击。可作为后续增强，非阻断项。

---

## 六、向后兼容性

- **proto 加字段**：proto3 天然兼容，旧节点启动时 startup_id=0，新节点能识别（0 视为"未提供，跳过校验"）
- **连接池**：阶段 2 可与现有 getCurrentClient 并存过渡（先共存后替换）
- **Draining 状态**：旧节点的 watch 不认识 Draining 会走 default 分支，需确保 handleKey 对未知状态值不 panic（已有 default 处理）
- **灰度发布自身**：本改造需随新版本发布；新老版本混部期间，新版本的 drain 能力只对都升级后的节点生效

---

## 七、风险与边界

| 风险 | 说明 | 缓解 |
|---|---|---|
| 连接泄漏 | 连接池无精确引用计数，可能累积空闲连接 | LRU + 定时清理（如 5min 未用则关） |
| 重新定位风暴 | 节点重启瞬间大量调用方同时重新定位 | 阶段 5 缓冲 + 退避 |
| drain 期间新流量落空 | 若新版本未起就 drain 老版本，新流量无处可去 | 运维约束：drain 前必须确认新版本就绪 |
| AtomAutoData 跨节点重建 | 阶段 5 的状态恢复依赖持久化数据完整 | 需业务保证 SetAtomData 可靠 |
| etcd watch 断线丢事件 | drain 状态变更靠 etcd watch 传播，分区可能丢失 | 独立审计项 E5，需配合 revision 续传 |

---

## 八、与审计问题的关系

本方案前置解决了以下审计发现：
- **ID 寻址断裂**（新发现，本文档核心）：current 切换导致存量 ID 失效
- **E6 remoteCosmos 只增不删**：drain 退出时配合清理 map
- **E2 current 单值无法灰度**：本方案用 Draining 状态 + 路由 fallback 替代
- **E3 tryKillingRemote 被注释**：用 drain 指令替代粗暴 kill

---

## 九、验证方案

> 核心矛盾：这套改造的风险点是**分布式状态**（ID 寻址、连接生命周期、版本切换），
> 这类 bug 的特征是——单测能覆盖逻辑分支，但**真正的故障只在多节点 + 网络时序 + 并发下才暴露**。
> 所以验证必须分层，每一层解决不同的问题。

### 9.0 测试基础设施现状评估（改造前必须摸清的底数）

| 能力 | 现状 | 可用性 |
|---|---|---|
| 单节点完整 CosmosProcess | `newTestCosmosProcessAsClusterNode`（含 element/atom/OnBoot/OnStartUp） | ✅ 已有 |
| **双节点 loopback gRPC** | `newTestCosmosProcessSimulateCluster`（手动接 remote 表，**绕过 etcd**） | ✅ 已有，覆盖 Get/Spawn/Sync/Async |
| 测试 element/atom | `ForTestAtomos`（proto 生成，带 Greeting/SayHello/haltNotify） | ✅ 已有 |
| **etcd mock / embedded** | **完全没有**；`util_etcd_test.go` 直连 `localhost:2379` | ❌ 最大缺口 |
| 版本切换端到端 | `TestSimulateUpgradeCosmosNode`（nodes_test.go:197）**整套被注释** | ❌ 需复活 |
| 死锁场景 | `base_atomos_deadlock_test.go` 3 个 + `nodes_test.go` 1 个 **全注释** | ❌ 需复活 |

**结论**：RPC 层验证现在就能做；etcd/集群/版本切换层验证**必须先补 embedded etcd 基础设施**。

### 9.1 第一层：单元测试（拦截逻辑错误，CI 必跑，零外部依赖）

**目标**：证明纯逻辑分支正确。快、稳定、可重复，是 CI 第一道门。

| 测试用例 | 验证什么 | 对应阶段 | 依赖 |
|---|---|---|---|
| `refresh()` 在 Started/Draining/Stopping 混合版本下选路 | drain 时路由正确绕开 Draining、优先 Started | 阶段 4 | 无（直接构造 version map） |
| 连接池 `getCli(addr)`：首次建连、同地址复用、空闲清理 | 连接池取/建/LRU 清理 | 阶段 2 | 无（in-process gRPC 或 mock） |
| `cosmosRemoteVersion` setDisable 后 getCli 行为 | Close 后置 nil、可重拨 | 阶段 0 | 无 |
| startupID 比对：不匹配返回"引用过期" | 引用失效检测 | 阶段 3 | 无 |

### 9.2 第二层：双节点集成测试（拦截 RPC 层回归，CI 可跑）

**目标**：证明 ID 寻址改造后，远程调用的端到端正确性——**尤其是 current 切换不断裂存量**。

复用 `newTestCosmosProcessSimulateCluster`（绕过 etcd），扩展 `cosmos_remote_test.go`：

| 测试场景 | 验证什么 | 改造前后预期 |
|---|---|---|
| V1 spawn atom → player 持有 ID → current 切到 V2 → player 调用 | **核心：current 切换不断裂存量** | 改造前：❌ 断裂（ErrAtomNotExists）；改造后：✅ 仍路由到 V1 |
| 双向 RPC（source↔target） | 现有只测单向 | 补齐 |
| 不存在的 atom → ErrAtomNotExists | 错误码正确 | - |
| kill atom 后调用 → 错误处理 | 生命周期边界 | - |
| startupID 过期 → 重新定位成功 | 阶段 3 引用失效重定位 | - |

> **关键实践**：改造前先写"current 切换后存量调用断裂"的**失败测试**（证实现状 bug），
> 改造后让它**通过**（证明修复）。这是测试驱动的核心证据。

### 9.3 第三层：etcd 集成测试（拦截集群层，需补基础设施）

**目标**：证明版本锁、节点发现、drain 状态广播、lease 过期的正确性。

**现状缺口**：没有 embedded etcd、没有 mock。必须先补，两个选择：

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **A（推荐）** | 引入 `go.etcd.io/etcd/tests/v3/integration` embedded etcd | 真实 etcd 行为（lease/watch/txn 全保真） | 引入测试依赖、启动秒级 |
| B | 抽接口注入 fake | 快、无外部进程 | fake 行为可能与真实 etcd 不一致（尤其时序），**测出来的可能不是生产行为** |

**强烈建议 A**——分布式系统的 mock 最容易"测了个寂寞"，embedded etcd 才能暴露真实的 lease 过期、watch 丢事件、txn 竞态。

补好后复活注释里的测试（现成模板）：

| 复活的测试 | 来源 | 验证什么 |
|---|---|---|
| `TestSimulateUpgradeCosmosNode` | nodes_test.go:197 | 版本切换/热更端到端（老版本退出、新版本接手） |
| `TestSimulateTwoCosmosNodeRPC` | nodes_test.go:375 | 双节点 RPC 全链路（几乎整个 remote service） |
| `TestSimulateTwoCosmosNode_FirstSyncCall` | nodes_test.go:9 | 跨节点死锁场景 |

**新增 etcd 专项测试**：

| 测试用例 | 验证什么 |
|---|---|
| drain 指令 → 节点状态变 Draining → 其他节点 watch 感知 → 路由刷新 | drain 状态广播链路 |
| drain 后新 Spawn 落到新版本（Started） | 路由 fallback |
| lease 过期（模拟 keepalive 断开）→ 节点被摘除 | lease 驱逐 |
| 版本锁 CAS 并发竞争 | 阶段 0 的 txn 正确性 |
| drain 超时兜底 → 强制 Stopping | deadline 生效 |

### 9.4 第四层：故障注入 / 混沌测试（拦截生产级故障，预发布环境）

**目标**：覆盖单测和集成测试到不了的"时序 + 网络 + 崩溃"组合。不在 CI，预发布专项。

| 场景 | 验证什么 |
|---|---|
| drain 期间 kill V1 进程 | 调用方重新定位到 V2，不永久卡死 |
| etcd 网络分区 30s 后恢复 | watch 不丢 drain 状态变更，无幽灵节点 |
| drain 超时兜底 | deadline 到达后强制 Stopping |
| 并发 drain + 新 Spawn | 无竞态导致新流量误落 draining 节点 |
| V1/V2 同时 drain | 所有节点都 draining 时的降级（不死锁） |
| drain 中连接池被 Close | 存量调用方的重连/重定位 |

### 9.5 第五层：灰度发布本身（生产验证）

改造上线时本身就是一次灰度——新版本（带改造）与老版本（不带）混部：

1. 发一个新版本节点，确认与老版本集群正常互通（proto 兼容）
2. 对新版本发 drain 指令，确认老版本接手新流量、新版本存量归零退出
3. 全量切换后，确认所有调用方 ID 寻址正确

### 9.6 各阶段对应的验证门禁

每个阶段实施完成后，必须通过的验证才能进入下一阶段：

| 阶段 | 必须通过的验证层 | 门禁理由 |
|---|---|---|
| 阶段 0（getCli bug） | 第一层 + 第二层 | 连接生命周期是后续所有阶段的基础 |
| 阶段 1-2（ID 寻址） | 第二层（current 切换不断裂测试） | 这是改造的核心价值，必须先证明确效 |
| 阶段 3（startupID 校验） | 第二层（过期重定位测试） | 引用失效处理不能卡死调用方 |
| 阶段 4（Draining） | 第三层（etcd 集成）+ 第四层（混沌） | drain 状态广播+故障恢复只能靠集成/混沌 |
| 上线 | 第五层（灰度） | 生产最终验证 |

### 9.7 可靠性保障（测试之外）

测试只能证明"已想到的场景没问题"。生产可靠性还需要：

| 保障 | 手段 |
|---|---|
| **可观测** | drain 状态、连接池大小、重新定位次数、存量 atom 计数全部加 metrics/log，运维能看到 drain 进度 |
| **可回滚** | drain 指令可逆（Draining→Started），发现异常能取消；改造有 feature flag 可关闭 |
| **渐进释放** | 阶段 0 先单独发布 → 阶段 1-3 再发布 → 阶段 4 最后开 |
| **兜底超时** | drain deadline、连接池清理周期、重新定位重试上限——任何环节都不会无限卡死 |

---

## 十、实施路线图（建议优先级）

1. **先补第三层基础设施（embedded etcd）**——验证 etcd 改动的前提，最大缺口
2. **第二层先写"current 切换断裂"的失败测试**——改造正确性的核心证据，现在就能写
3. **阶段 0（getCli bug）单独验证发布**——独立于 drain，是后续所有阶段的前置
4. 阶段 1-3（ID 寻址）一起实施验证
5. 阶段 4（drain）最后实施，配合混沌测试
