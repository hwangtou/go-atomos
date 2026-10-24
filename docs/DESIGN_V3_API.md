# AtomOS v3 接口重构设计

> 状态：草案（v3 分支）
> 日期：2026-10-23
> 范围：根包 `atomos` 公开 API 的破坏性重构。v3 允许 breaking change，但每项变更必须附迁移路径。

## 0. 背景与目标

v2 的公开 API 面存在系统性问题，集中表现为：

1. **AI/工具不友好**：全局单例、不可 mock 的接口、裸 bool/`...any` 签名，使代码生成与测试生成不可靠。
2. **使用者测试困难**：`ID`/`Logging` 等核心接口含 unexported 方法，外部包无法实现，mock 无从下手。
3. **内部实现泄漏**：`ForTest*` 测试设施、util 工具大杂烩、gRPC 调优常量等全部混入生产导出面。
4. **风格不统一**：同一框架内三种参数传递形态、新旧 `interface{}`/`any` 混用、命名前缀不一致。

v3 目标：

- **可实例化**：去掉全局单例语义，测试可并行、可隔离。
- **可实现**：所有公开接口外部包均可实现（mock 友好），内部扩展点下沉到非公开接口。
- **可静态检查**：消灭裸 bool 与 `...any` 参数，参数体系统一为封闭的功能选项接口。
- **惯用 Go**：错误处理支持 `errors.Is/As`；`fmt.Stringer` 等标准接口正确实现。
- **面收敛**：导出面只保留框架契约，内部物移入 `internal/` 或取消导出。

## 1. 进程入口与全局状态（P0）

### 1.1 现状

- 四个入口：`Main` / `MainForConfigFile`（纯别名，app_main.go:58）/ `MainForWorkingPath` / `MainForTest`，行为差异仅存在于注释。
- 全局单例：`sharedCosmosProcess` + `sync.Once`（app_main.go:200），包级 `var app *App`（app_main.go:12）。
- 全局 setter：`SetMessageTimeoutTracer` / `MuteKeepaliveLog` / `SetIDTrackerDebug`（variable.go:12,21,29），注释自认"新代码应直接设在 CosmosProcess 实例上"。
- 可变包级 var：`drainCheckInterval`、`drainDefaultDeadline`（cosmos_process.go:419-420）。

### 1.2 v3 方案

**单一入口 + 功能选项**：

```go
// v3 唯一进程入口
func Run(runnable *CosmosRunnable, opts ...RunOption)

type RunOption func(*runConfig)
func WithConfigFile(path string) RunOption   // 默认读 --config flag
func WithWorkingPath(path string) RunOption
func WithStandalone() RunOption              // 等价于现在的 --standalone / Docker 模式
```

- 删除 `MainForConfigFile`（别名无存在价值）。
- `MainForTest` 移出主包：`atomostest.Run(t, runnable, opts...)`（新测试工具包，见 §7）。
- `Main` 保留一个版本作为 `Run` 的薄封装并标注 `Deprecated`，v3.1 删除。

**实例化配置**：

- `MessageTimeoutTracer` / `KeepaliveLog` / `IDTrackerDebug` 改为 `CosmosRunnable` 的 builder 方法（`SetMessageTimeoutTracer(v, d)` 等），随 `CosmosProcess` 实例走，删除 variable.go 全局 setter。
- `drainCheckInterval` / `drainDefaultDeadline` 改为 `Config` 字段（带默认值），删除包级 var。
- `SharedCosmosProcess()` 删除；少数内部引用改为显式传 `*CosmosProcess`。

**迁移**：`Main(r)` → `Run(r)`；`MainForWorkingPath(r, p)` → `Run(r, WithWorkingPath(p))`；`MainForTest(r, t)` → `atomostest.Run(t, r)`。

## 2. 可实现接口：拆分公开面与内部扩展（P0）

### 2.1 现状

含 unexported 方法的公开接口，外部包**无法实现**，mock 测试无从谈起：

| 接口 | unexported 方法 | 位置 |
|---|---|---|
| `ID` | `asyncSet` / `asyncCallback` / `getGoID` | id.go:40,43,62 |
| `Logging` | `coreInfo` / `coreError` / `coreFatal` | atomos_logging.go:17-22 |
| `LoggingService` | 3 个 | logging_atomos.go:14-16 |
| `MailboxHandler` | 全部（外部完全不可用却导出） | mail.go:131-135 |

`ID` 是万恶之源：`SelfID`/`AtomSelfID`/`ElementSelfID`/`CosmosNode`/`Element` 全部嵌入 `ID`，整个句柄体系都不可实现（AUDIT_TODO.md D2）。

### 2.2 v3 方案：公开接口纯净化 + 内部接口下沉

```go
// 公开接口：只含导出方法，外部可实现、可 mock
type ID interface {
    GetIDInfo() *IDInfo
    String() string
    Cosmos() CosmosNode
    State() BaseAtomosState
    IdleTime() time.Duration
    SyncMessagingByName(callerID ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (proto.Message, error)
    AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(proto.Message, error), ext ...ArgsForBaseAtomos) error
    DecoderByName(name string) (in, out MessageDecoder)
    Kill(callerID ID, ext ...ArgsForBaseAtomos) error
    SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext ...ArgsForBaseAtomos) error
}

// 内部扩展：框架实现提供，用户永远不需要关心
type idInternal interface {
    ID
    asyncSet(callback func(proto.Message, error)) (startupID, callbackID uint64)
    asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err error)
    getGoID() uint64
}
```

框架内部需要内部方法处做类型断言（断言失败返回 `ErrFrameworkInvalidID`——用户传了自制 mock 给框架内核路径，这本身就是编程错误，应显式失败而非静默）。

同样处理：

- `Logging`：公开面只留 `io.Writer + SetLevel + Debug/Info/Warn/Error/Fatal`；`coreInfo/coreError/coreFatal` 下沉到 `loggingInternal`。
- `LoggingService`：直接取消导出（`loggingService`）。
- `MailboxHandler`：直接取消导出（`mailboxHandler`）。

**配套**：官方提供 `atomostest` 包的 fake 实现（`atomostest.NewFakeID()` / `NewFakeLogging()`），用户不必手写 mock。

## 3. 错误语义：拥抱 `error` 与 `errors.Is/As`（P0）

### 3.1 现状

- 全框架返回具体类型 `*Error`（proto 消息），不支持 `errors.Is/As`；判等只能比 `err.Code`。
- 唯一特判 helper：`IsAtomExist()`（atomos.extension.go:154）。
- 错误码是单一 iota 块，靠"只能追加"注释维持跨进程兼容（error_code.go）。
- `*Error` 为 nil 时调用方法需谨慎；`Error` 字段导出可变，同时承担 RPC 序列化 + `sql.Scanner`/`driver.Valuer` 双重职责。

### 3.2 v3 方案

**公开 API 返回 `error` 接口**，具体类型仍为 `*Error`（proto wire 格式不变，跨节点兼容不受影响）：

```go
// 哨兵错误：支持 errors.Is
var (
    ErrAtomNotExist       = &Error{Code: ErrCodeAtomNotExist, ...}
    ErrAtomSpawningAnExistedAtom = ...
    ErrCosmosNodeDraining = ...
    // ... 按现有错误码全量生成
)

// Error 实现 Is：按 Code 匹配
func (e *Error) Is(target error) bool

// 工具函数
func CodeOf(err error) int64   // 取错误码，非 *Error 返回 ErrCodeUnknown
```

**错误码显式编号冻结**：iota 块改为显式字面值并加 CI 检查（类似 protobuf 的保留号策略），消灭"顺序即协议"的脆弱性：

```go
const (
    ErrCodeOK                 int64 = 0
    ErrCodeAtomNotExist       int64 = 1
    // ... 现有值全部显式写出，永不修改
)
```

**迁移**：

- 用户侧 `err.Code == ErrXxx` → `errors.Is(err, atomos.ErrXxx)`。
- 生成器同步改造：生成的 Element/Atom 接口返回值从 `*Error` 改为 `error`（v3 生成器与 v3 框架绑定发布）。
- `*Error` 继续导出（用户需要构造业务错误），但 `NewError/NewErrorf` 返回 `*Error` 的签名保留。

### 3.3 实施状态（v3.0-alpha2 已落地）

- 错误码：`error_code.go` 全部 131 个常量已改为显式字面值（0–130），`error_code_snapshot_test.go` 以硬编码快照 + 唯一性校验冻结（CI 级别防改号）。
- 旧 `ErrXxx` 常量全量改名 `ErrCodeXxx`（AST 级标识符重写，注释/字符串/注释掉的测试不受影响）。
- `error_sentinel.go`：`(*Error).Is` 按 `Code` 匹配；20 个哨兵变量（`ErrAtomNotExists`、`ErrAtomIsRunning`、`ErrCosmosNodeDraining` 等，沿用旧 `Err` 名占用位）；`CodeOf(err)`（非 `*Error` 返回 `ErrCodeUnknown = -1`，对 typed-nil 安全）；`IsAtomExist` 保留但标记 Deprecated。哨兵为按需清单而非全量 130 个，后续按用户场景增补。
- 公开 API 全部改返回 `error`：`ID`/`CosmosNode`/`Element` 四方法族、`CosmosMainScript`、`ElementLoader`、`AtomAutoData`/`ElementAutoData`、`AtomosAcceptWormhole`/`AtomosRecover`、`MessageHandler`/`MessageDecoder`/`ElementSpawner`/`AtomSpawner`、`CosmosRunnable` 的 hook 与 Setter、`Config.Check`、`App`、`CosmosProcess.Start/Stop/Drain`、`WithRebind`、Task/Messenger 族等。内核路径（mailbox、task manager、remote service）仍用 `*Error`，边界经 `asError`（入）/`errOrNil`（出）转换，杜绝 typed-nil 外泄；`TestPublicAPISuccessReturnsUntypedNil` 锁定成功路径必归真 nil。
- 生成器 `protoc-gen-go-atomos` 模板同步改造，`for_test.proto` fixture 已重新生成；`cmd/atomos-vet` testdata 同步适配。
- 门禁：`build ./...` + `vet ./...` + `GOOS=windows build ./...` + 全量测试通过。

## 4. 参数与签名统一（P1）

### 4.1 现状

- 同一框架三种传参形态：`ext []ArgsForBaseAtomos` 切片（id.go:32）、`args ...ArgsForBaseAtomos` 变参（cosmos.go:33）、`args ...any`（element.go:20）。
- 裸 bool：`GetAtomID/SpawnAtom(..., fromLocalOrRemote bool, ...)`（element.go:20,32，`ElementRemote` 实现里干脆 `_` 丢弃）；`AddElementImplementation(i, setSpawn bool)`（cosmos.go:98）。
- 命名不一致：`NewArgBaseAtomosWithAny` 带 `New` 前缀且导出同名结构体，其余 9 个构造器无前缀（args.go:37）。
- `IDTracker.ToString()` 不实现 `fmt.Stringer`（atomos_id_tracker.go:182）。
- `interface{}` / `any` 新旧混用。
- `ArgTaskCancelCallback` doc 示例与实际签名不符（args.go:67-72）。

### 4.2 v3 方案

1. **参数形态统一为变参功能选项**：全部改为 `ext ...ArgsForBaseAtomos` / `ext ...ArgsForTask`（切片形态删除）。封闭接口设计保留（ArgsForBaseAtomos 等含 unexported 方法是刻意的，防止用户乱造参数），属正确设计，文档中说明理由。
2. **裸 bool 改为类型化参数**：
   - `fromLocalOrRemote bool` → 并入 Args 体系：`ArgBaseAtomosFromLocalOnly()` / `ArgBaseAtomosFromRemoteOnly()`（缺省自动路由）。`Element.GetAtomID/SpawnAtom` 签名中删除该参数。
   - `AddElementImplementation(impl, setSpawn bool)` → 拆为两个方法 `AddElementImplementation(impl)` 与 `AddElementImplementationWithSpawn(impl)`，或改选项。
3. **命名统一**：`NewArgBaseAtomosWithAny` → `ArgBaseAtomosAny(v any)`，删除导出结构体 `ArgBaseAtomosWithAny`（改为非导出）。
4. `ToString()` → `String()`（实现 `fmt.Stringer`）。
5. 全库 `interface{}` → `any`。
6. 修正 args 文档示例；`ArgType` 枚举显式编号。
7. `CosmosMain()` 返回 `*CosmosLocal` 具体类型 → 返回 `CosmosNode` 接口（id.go:85），需要本地能力处保留 `CosmosLocal` 类型断言辅助。

### 4.3 实施状态（v3.0-alpha3 已落地）

- **参数形态**：`ID.SyncMessagingByName/AsyncMessagingByName/Kill/SendWormhole` 与 `BaseAtomos/BaseRemote` 的 `Push*` 全部改为 `ext ...ArgsForBaseAtomos`，切片形态删除；`Element.GetAtomID` 的 `args ...any` 改为 `...ArgsForBaseAtomos`。注意迁移坑：显式传 `nil` 在可变参数下变成"含一个 nil 的切片"，会被 helper 判为非法参数（原先 nil 切片=无参），调用点须改为省略该参数。
- **裸 bool 消除**：`Element.GetAtomID/SpawnAtom` 的 `fromLocalOrRemote bool` 删除，新增 `ArgBaseAtomosFromLocalOnly()`/`ArgBaseAtomosFromRemoteOnly()`（args_impl.go ArgType 6/7）。实现语义说明：该标记的实际作用是"用户发起的调用，框架需跟踪 IDTracker 引用"（ElementRemote 仍忽略，路由在 cosmos 层已定）；缺省（无标记）= 框架内部调用不跟踪，对应原 `false` 行为。`CosmosRunnable.AddElementImplementation(impl, setSpawn bool)` 拆为 `AddElementImplementation(impl)` + `AddElementImplementationWithSpawn(impl)`，内部共用 `addElementImplementation`。
- **命名与细节**：`NewArgBaseAtomosWithAny` → `ArgBaseAtomosAny(v any)`，导出结构体 `ArgBaseAtomosWithAny` 取消导出；取消导出后须保留回读能力，新增 `ArgBaseAtomosAnyValue(arg) (any, bool)` 读取器（Mexico-server 验证发现真实消费场景：ElementLoader.Load 从 Spawn args 回读 `*testing.T`/`*testing.B` 识别测试上下文，见 args.go）；`IDTracker.ToString()` 删除、改 `String()`（fmt.Stringer，生成器为嵌入 ID+*IDTracker 的生成 ID 类型补显式 `String()` 消除 selector 歧义）；生产代码 `interface{}` 全改 `any`（生成的 pb 代码除外）；args.go 文档示例改可变参数形态并修正 `ArgTaskCancelCallback` 示例签名；`ArgType` 原本已是显式编号，无需改。`SelfID.CosmosMain()` 直接改返回 `CosmosNode`（影响面仅 3 个实现、无库内调用点，未采用降级方案）。

## 5. 导出面收敛（P1）

### 5.1 移入 `internal/` 或取消导出

| 符号 | 处置 |
|---|---|
| `ForTest*` 全家桶（for_test.proto 生成物、`GetForTestAtomosAtomID` 等） | 移到 `internal/testatom/`，测试专用。框架自身测试引用同步修改 |
| `ShouldArgumentClone` / `ConfigKeyLogStdout` / `GRPC* 调优常量` / `DefaultMailID` | 取消导出 |
| `NewBaseAtomos` / `BaseAtomosHolder` / `AtomLocal` 等句柄上的 `OnFnCallback(*atomosCallback)` 及全部 `On*` 框架回调方法 | 取消导出或移入内部接口（这些不是用户契约，是实现细节泄漏） |
| `InitCosmosProcess(..., logging appLogging, ...)`（导出函数收未导出类型，外部无法调用） | 取消导出 |
| `Util*`/`Path`/`CheckCosmosName`/`GetEnv*Key` 工具大杂烩 | 移入 `internal/util/`；确有用户价值的（如 `UtilDiffMaps`）不导出 |
| `remoteFakeSelfID` 对应物、`AtomRemoteInSourceProcess` 等 RPC 适配类型 | 取消导出 |

### 5.2 结构修正

- `MapGo[K,V]` 内嵌 `sync.RWMutex`（值拷贝即复制锁）→ 改为具名字段。
- `MainForConfigFile` 删除（见 §1）。
- `drainCheckInterval`/`drainDefaultDeadline` 可变成 const/Config（见 §1）。

## 6. 生成代码与文档（P1）

### 6.1 protoc-gen-go-atomos v3

生成器与框架 v3 绑定发布（同一 tag），生成物同步变更：

- 生成的业务接口错误返回 `*Error` → `error`。
- 生成的 ID 客户端类型调用签名适配 §4 的统一参数形态。
- 修复遗留项：G7（`noExport` 首字母缩写）、G9（Spawn 签名 input/output 角色反直觉，v3 统一为 arg 在 input、data 在 output 并写入文档）、Messenger 泛型精简（O7，如改动可控）。
- 生成器补 golden-file 测试，纳入 CI；消灭"插件版本与框架 API 脱节"（regen-proto SKILL.md 当前警告）的状态。

实施状态（v3.0-alpha3 部分落地）：错误返回 `error`（alpha2 已完成）与 §4 参数形态适配（变参 ext、生成的 ID 类型显式 `String()` 消歧）已落地；**golden-file 测试已建**（`protoc-gen-go-atomos/golden_test.go` + `testdata/golden/`，in-process protogen 驱动 + protoc descriptor set，覆盖显式 option 与前缀回退共 8 种方法分类，`-update` 刷新），尚未纳入 CI。G7/G9/Messenger 泛型精简未做。

### 6.2 文档

- 所有导出符号补**英文 godoc**（现有中文注释保留为内部实现注释或翻译，公开面统一英文，方便工具链与国际社区）。
- 核心入口配 godoc Example 函数（`ExampleRun`、`ExampleElementDeveloper` 等）——这是 AI 工具最高质量的参考材料。
- README 重写：quickstart、架构图、测试指南链接。
- 恢复 examples/（hello_world 基于 v3 API 重写）。

实施状态（v3.0-beta1 已落地）：核心公开符号（ID/SelfID/Element/CosmosNode/CosmosRunnable/WithID/WithRebind/Arg* 系列/错误哨兵）已统一英文优先 godoc，中文保留为翻译注记；根包新增 `example_test.go`（`ExampleCodeOf`、`ExampleArgBaseAtomosAnyValue`、`ExampleWithRebind`，均为不起进程的 testable Example）。README 已重写（quickstart/特性/测试指南/仓库布局），新增 AGENTS.md（构建测试命令、proto 生成禁忌、常见错误模式）。examples/hello_world 已基于 v3 API 重建并留在主 module（`go build ./...` 覆盖），含 proto/gen.sh/实现/进程级测试，二进制实际运行验证通过（spawn→SayHello→SIGINT 优雅退出）。`ExampleRun`/`ExampleElementDeveloper` 因需起进程未做成 Example，由 examples/hello_world 承担此角色。架构图未配（纯文字 README）。

## 7. 测试工具包 `atomostest`（P1，与接口重构互为依赖）

接口可实现化（§2）落地后，新建 `atomostest` 子包：

- `atomostest.Run(t, runnable, opts...)`：测试进程入口（替代 `MainForTest`）。
- `atomostest.NewFakeID()` / `NewFakeLogging()` / `NewFakeSelfID()`：官方 fake。
- 内存模拟节点（无 gRPC/etcd/fork）+ 虚拟时钟：作为后续阶段，设计文档另立。

实施状态（v3.0-beta1 已落地）：`Run` 已有；`NewFakeID`/`NewFakeLogging`/`NewFakeSelfID` 已实现并配单测（`atomostest/fake_*.go`、`fakes_test.go`）。FakeID 支持按消息名配置回复（`SetMessageReply`）、动态 handler（`SetSyncHandler`）与调用历史断言（`Calls`/`CallCount`/`ResetCalls`）；FakeSelfID 内嵌 FakeID 并提供 FakeLogging 日志记录、同步执行的 FakeTask、可设置的 CosmosMain/Config/AsyncCallbackPending 与计数式 KillSelf。包 doc 已声明边界：fake 仅用于测试用户业务逻辑，注入框架内核路径会被 `ErrFrameworkInvalidID` 显式拒绝。内存模拟节点与虚拟时钟仍按原计划另立设计。

## 8. 内部 RPC 协议与错误传输（P2）

- gRPC 状态码映射：`Error.Code` 映射到标准 `codes.Code`（NotFound/FailedPrecondition/...），业务详情仍放响应体内；使监控与拦截器可识别失败。跨版本兼容策略：v3 节点同时写两处，读侧优先读 status。
- `DrainNode` 等管理面 RPC 保留内嵌错误（管理面工具链友好优先）。
- 协议加 `version` 握手字段的取舍：v3 暂缓，记录在案。

实施状态（v3.0-rc1 已落地前两项，握手字段维持暂缓）：映射表与互转在 `atomos_grpc_status.go`——`grpcCodeOfErrCode`（正向归类：不存在类→NotFound，状态拒绝类→FailedPrecondition，参数类→InvalidArgument，超时类→DeadlineExceeded，panic/内部类→Internal，其余→Unknown）、`grpcStatusFromError`（服务端双写：`*Error` proto 进 status details，响应体 Error 字段照填）、`grpcErrorToError`（客户端读侧：details 优先还原含 CallStacks 的完整 `*Error`，缺失时按 status code 反映射为代表性错误码，如 Unavailable→ErrCodeCosmosRemoteConnectFailed）。服务端接入点为 7 个调用类 RPC 的 wrapper（`callRspWithStatus`）；客户端接入点为 `BaseRemote.PushSyncMessage/PushAsyncMessage/Kill` 与 `ElementRemote.GetAtomID/SpawnAtom`。管理面 RPC（TryKilling/DrainNode/GetElementInfo/ElementBroadcast/AsyncOnMessageCallback）保持仅内嵌，代码注释已写明取舍。单测 `atomos_grpc_status_test.go` 覆盖映射表、status 往返（Code/CallStacks 不丢失）与反映射回退；`cosmos_remote_test.go` 的 raw-stub 集成测试已改为新契约（非 OK status + details 还原），兼作双写端到端覆盖。

## 9. 实施顺序

| 阶段 | 内容 | 验证 |
|---|---|---|
| v3.0-alpha1 | §1 入口与全局状态、§2 接口拆分、§5 导出面收敛 | 全量测试通过；`go vet` + atomos-vet 通过 |
| v3.0-alpha2 | §3 错误语义（`error` 返回值 + `errors.Is` + 错误码冻结） | 全量测试适配；错误码 CI 检查脚本 |
| v3.0-alpha3 | §4 参数统一 + §6.1 生成器 v3 + golden 测试 | for_test.proto 重新生成并编译运行（golden 测试已建，CI 接入待做） |
| v3.0-beta1 | §6.2 文档与 examples、§7 atomostest 基础 | hello_world 示例跑通 |
| v3.0-rc1 | §8 协议改进（如纳入）、兼容性冒烟（v2/v3 节点共存灰度验证） | 双节点集成测试（TestCosmosRemote_* 双进程 gRPC 套件全绿；v2/v3 兼容证据见 §10） |

## 10. 兼容承诺

- v3 不保证源码兼容；提供本节各项迁移说明，并在 CHANGELOG 逐条列出。
- **线协议兼容**：`atomos.proto` 字段编号冻结策略不变；v3 节点与 v2 节点跨版本调用的兼容矩阵在 rc 阶段验证（IDInfo/Error/args 的 wire 格式未变，预期互通，startup_id/instance_id 语义不变）。
- 生成代码：用户需用 v3 版 protoc-gen-go-atomos 重新生成业务代码。

### 10.1 rc1 验证结果

对照基线分支 `preview/20261023`（即 v2 线）：

1. **wire 格式不变**：`git diff preview/20261023..v3 -- atomos.proto atomos.pb.go atomos_grpc.pb.go` 为空，三个文件逐字节一致。另用当前工具链（protoc 35.1 + protoc-gen-go v1.36.10）重新生成 `atomos.pb.go` 与仓库提交版本 `diff` 为空，descriptor 字节级一致。IDInfo/Error/args 的 wire 格式与 startup_id/instance_id 语义不变成立。
2. **错误码不变**：alpha2 把 iota 改为显式数值后，`error_code_snapshot_test.go` 锁定全部 131 个码。rc1 做了一次性交叉校验：将 `preview/20261023:error_code.go` 的 iota 常量编译打印（130 个值，OK+129 个错误码），与 v3 快照逐一比对——**全部一致，无一偏移**；v3 唯一新增为尾部追加的 `ErrCodeFrameworkInvalidID=130`（符合 append-only 规则）。
3. **双写/读取策略的跨版本行为**：
   - v3 服务端 → v2 客户端：调用类 RPC 业务失败时 v2 客户端收到非 OK gRPC status。v2 的 `base_remote.go`/`element_remote.go` 调用点在 `er != nil` 分支直接返回包装后的 `ErrCosmosRemoteResponseInvalid`，**不触碰 nil 响应体、无 panic**，调用失败可按原有逻辑重试。注意双写下 body.Error 非空与非 OK status 同生同灭，因此 v2 侧读 body.Error 精确业务码的路径被完全遮蔽（ErrAtomNotExists 等远程错误在 v2 侧一律变为 ErrCosmosRemoteResponseInvalid）——这是 §8 双写策略的已知取舍，灰度时需排查 v2 依赖远程调用精确错误码做分支逻辑的调用点。
   - v2 服务端 → v3 客户端：v2 永不返回非 OK status，v3 客户端走原有 body.Error 路径，逐字节兼容。
   - v3 ↔ v3：details 优先，Code/Message/CallStacks 完整往返（`TestGRPCStatusRoundTrip` + `TestCosmosRemote_InstanceMismatchOnRespawn` 等集成测试覆盖）。
   - 双节点集成：`TestCosmosRemote_*`（双真实 CosmosProcess 进程间 gRPC，无 etcd）全部通过，含 spawn/消息/Kill/instance mismatch/instance_id=0 拒绝/cosmos 层端到端。
