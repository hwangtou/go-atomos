# 混部灰度排查指南：v2/v3 错误码遮蔽问题

适用对象：**正在或计划混跑 v2 与 v3 节点的项目**（同一 cosmos 内 v2/v3 节点
互通）。纯 v3 集群不受影响。状态：2026-10-24 定稿；对应设计依据
`docs/DESIGN_V3_API.md` §8（双写）与 §10.1（兼容矩阵第 3 条已知取舍）。

## 问题是什么

v3 服务端对调用类 RPC（SyncMessagingByName / AsyncMessagingByName /
SpawnAtom / KillAtom / GetAtomID / GetIDState / GetIDIdleTime）的业务失败
**双写**：响应体照旧内嵌 `*Error`，同时返回非 OK 的 gRPC status（完整错误在
status details 里）。

gRPC 库在非 OK status 时**丢弃响应体**，于是 v2 客户端（只读响应体）在
`er != nil` 分支拿不到任何精确错误码，统一得到包装后的
`ErrCosmosRemoteResponseInvalid`。

一句话：**v2 调用方 → v3 服务端的远程业务失败，错误码从“精确”退化为
“一律连接类失败”。本地调用与 v3→v3 调用不受影响。**

v2 → v2 与 v3 客户端 → v2 服务端逐字节兼容，不在本文范围。

## 谁需要排查、在哪排查

在 **v2 业务/生产仓库**排查（不是本框架仓库）。只要 v2 代码里存在"对
**跨节点调用**返回的错误码做分支"的调用点，就需要过一遍下面的流程。

## 排查步骤

### 第一步：机械扫描

扫描 v2 代码中所有远程调用点——`Get*AtomID` / `Spawn*Atom` / 生成 client
方法 / `SyncMessagingByName`——其后紧跟的错误判断。grep 模式：

```sh
# v2 风格（*Error 结构体字段比较）
grep -rn --include='*.go' -E 'err\.Code == atomos\.Err|\.Code == Err[A-Z]' ./...

# 哨兵/码比较（v2 后期与 v3 风格）
grep -rn --include='*.go' -E 'err == atomos\.Err[A-Z]|errors\.Is\(err, atomos\.Err|CodeOf\(err\) ==' ./...
```

命中的行若作用于**远程调用返回值**（判断依据：调用目标 ID 来自
`GetElementID`/`GetAtomID`/生成的 client，且目标可能落在其他节点），进入
第二步。纯本地 actor 间的调用点可直接划掉。

### 第二步：逐点分类

| 分类 | 判定 | 混部影响 | 处理 |
|------|------|----------|------|
| A. 仅日志/监控 | 错误码只进日志或打点 | 无害（码变粗） | 可放过 |
| B. 重试逻辑 | NotFound→再 spawn 一次、busy→退避重试等 | 仍会重试（误判为连接失败通常等价），但要确认不会因"永远 Invalid"而无限重试 | 核对重试上限；无上限的加上限 |
| C. 业务分支 | NotFound→流程 A，NotRunning→流程 B 之类 | **行为改变：分支永远走不进去** | 必须处理（见第三步） |

### 第三步：C 类处理手段（按优先级）

1. **调用方升级到 v3**：v3 客户端从 status details 还原完整 `*Error`
   （Code/Message/CallStacks），精确码恢复。这是根治。
2. **分支逻辑下沉到被调方**：在 v3 节点的 handler 内部完成判断，用不同的
   返回消息（而非错误码）表达结果，调用方不再依赖错误码。
3. **兜底降级**：混部窗口期内该功能接受退化（例如统一按失败处理），v2
   节点清零后恢复。

### 第四步：产出清单并关闭

产出《混部错误码排查清单》：`调用点(文件:行) → 分类 → 处理决定 → 负责人`。
灰度上线前作为前置交付物评审。当 §8.1 version 握手 Phase 0 落地后，用
`atomos_remote_protocol_version{node=...}` 确认集群内 v2 节点清零，清单即可
关闭。

## 快速自检问答

- **我们全 v3 了还要看吗？** 不用。本文只针对混部窗口。
- **v2 服务端 → v3 客户端呢？** v2 永不返回非 OK status，v3 客户端回落到
  读响应体，逐字节兼容。
- **怎么提前发现踩坑？** 混部期监控 v2 侧 `ErrCosmosRemoteResponseInvalid`
  占比突增、且伴随本应分流的功能未分流，即本文描述的症状。
