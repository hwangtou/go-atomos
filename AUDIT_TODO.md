# go-atomos 框架审计问题跟踪

基于完整审计报告，逐项核验新仓库（`preview/20260728`）状态。

状态图例：✅ 已修复 | ❌ 仍存在 | ⚠️ 部分修复 | ➖ 不适用/设计取舍

---

## 第一批：P0/P1 致命/严重（已在 commit 8abc6c9 修复，13 项）

| # | 问题 | 文件 | 状态 |
|---|---|---|---|
| A1 | waitPop cond.Wait 不在 for 循环 | mail.go | ✅ 已修复 |
| A2 | 版本锁清空列表（不报错） | cosmos_etcd_version_lock.go | ✅ 已修复 |
| A3 | coreFatal 写入 CoreErr 级别 | atomos_logging.go | ✅ 已修复 |
| A4 | etcd watchCluster goroutine 无 recover | cosmos_etcd.go | ✅ 已修复 |
| A5 | asyncCallbackMap 远程无回复永久泄漏 | base_atomos.go | ✅ 已修复 |
| A6 | taskMailbox.handleAtomosMail 无 recover | task_mailbox.go | ✅ 已修复 |
| A7 | setHalted 持锁调 hook + 发 channel | base_atomos.go | ✅ 已修复 |
| A8 | InitBaseAtomosMailSync if/else 相同 | base_atomos_mails.go | ✅ 已修复 |
| A9 | mailbox loop panic 丢邮件 | mail.go | ✅ 已修复 |
| A10 | cosmosRemoteVersion.check() 持锁拨号 | cosmos_remote.go | ✅ 已修复 |
| A11 | e.names 遍历与并发 Remove | element_local.go | ✅ 已修复 |
| A12 | etcd keepalive 重试后无谓 sleep | cosmos_etcd.go | ✅ 已修复 |
| A13 | unloadClusterLocalNode etcd close 注释 | cosmos_process.go | ✅ 已修复 |

---

## 第二批：P2 中等（已修复）

| # | 问题 | 文件 | 状态 | 说明 |
|---|---|---|---|---|
| B1 | IDTracker.fromOld 热更新未验证 | atomos_id_tracker.go:24 | ✅ 已修复 | 保留 fromOld 语义，替换 TODO 为完整设计文档（配合 B5 的 Release 幂等保证安全） |
| B2 | logging Close 死代码 | app_logging.go / app.go | ✅ 已修复 | AppLoggingToFile/AppLoggingToConsole 加 Close；app.close() 调用 logging.Close() |
| B3 | 配置校验空实现 | config.go | ✅ 已修复 | ValidateSupervisor/CosmosNodeConfig 实现真实校验，Config.Check() 调用它们 |
| B4 | MapGoWithRefCount.Put 语义错误 | util_sync_map.go | ✅ 已修复 | Put 改为覆盖语义，新 key 设 ref=1，已存在不自增 |
| B5 | IDTracker.Release 二次调用无幂等保护 | atomos_id_tracker.go | ✅ 已修复 | Release 后置 nil manager，二次调用安全 no-op |
| B6 | logging 并发：无锁写文件模型 | app_logging.go | ⚠️ 接受 | buf 竞态已修；多写者靠 mailbox 串行化兜住，模型属设计取舍 |
| B7 | 三套启动入口行为分裂 | app_main.go | ⚠️ 接受 | MainForConfigFile 已合并；WorkingPath/Test 的差异属不同场景需要 |

---

## 第三批：P3 代码质量（已修复）

| # | 问题 | 文件 | 状态 | 说明 |
|---|---|---|---|---|
| C1 | AddPanicStack 无 nil 防护 | atomos.extension.go | ✅ 已修复 | 加 if x==nil return nil |
| C2 | IDInfo.IsEqual 无 nil 防护 | atomos.extension.go | ✅ 已修复 | 加 x/r 的 nil 检查 |
| C3 | ioutil 已废弃 | app_env.go / util_file_unix.go | ✅ 已修复 | 改为 os.ReadFile/WriteFile |
| C4 | pidPerm = 0444 只读 | app.go | ✅ 已修复 | 改为 0644（owner-writable） |
| C5 | error_code iota/显式混用 | error_code.go | ✅ 已修复 | 显式段改为 iota 续接，单一序列无冲突 |
| C6 | releaseBaseAtomosMail 空函数 | base_atomos_mails.go | ✅ 已修复 | 加注释说明有意依赖 GC（对象池有 use-after-reuse 风险） |

---

## 第四批：设计层面（不修复，记录）

| # | 问题 | 状态 | 说明 |
|---|---|---|---|
| D1 | ElementConstructor/AtomConstructor 返回同一 Atomos | ➖ | 设计取舍，类型安全靠运行时断言 |
| D2 | ID 接口混入 internal 方法 (asyncSet 等) | ➖ | 是 B 系 remote asyncSet 桩的根因，但拆分影响面大 |
| D3 | 无 metrics / 无 pprof | ➖ | 已加 IsHealthy()；metrics/pprof 需独立 HTTP server，范围大 |

---

## 已由新仓库修复（无需处理，仅记录）

- signal.Notify 全信号监听 → ✅ 改为 SIGINT/SIGTERM/SIGHUP/SIGQUIT
- redirectSTD TODO Memory Leak → ✅ 重新实现为直接赋值
- CVE 依赖 → ✅ grpc 1.79.3 / protobuf 1.36.10 / x/net 0.48.0 / etcd 3.6.4
- spawnLockMap Remove/Unlock 顺序 → ✅ 持锁时 Remove
- Persistence() if ok bug → ✅ 改为 if !ok
- asyncSet panic → ✅ 改为 return 0,0
- grpc.WithInsecure deprecated → ✅ 改为 insecure.NewCredentials()
- 包名 go_atomos → ✅ 改为 atomos
- detectDeadlock 死变量 → ✅ 已删除
- BaseAtomosWormhole 空接口 → ✅ 改为 IsWormhole()
- Halt/Stopping 字符串冲突 → ✅ Halt 返回 "Halted"
- 全局变量 → ✅ 改为 CosmosProcess 实例字段

---

## 结论

所有可修复的 P2/P3 问题已处理（B1-B5、C1-C6）。剩余 B6/B7/D1-D3 属设计取舍或需独立大改，标记为接受。
P0/P1 的 13 项已在 commit 8abc6c9 修复。
