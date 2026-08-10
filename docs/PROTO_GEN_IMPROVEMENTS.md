# protoc-gen-go-atomos 代码生成器优化

> 基于 `protoc-gen-go-atomos/atomos.go`（555 行）+ `dev_messenger.go` 的完整审阅

## 生成器工作原理

扫描 proto 的 `service` 定义，按**方法名前缀约定**区分 Element 方法 vs Atom 方法：
- `ElementSpawn` / `ElementXxx` → Element 级方法（落在 singleton element 上）
- `Spawn` / `Xxx` → Atom 级方法（落在 per-instance atom 上）

对每个 service 生成：接口定义、ID 类型、工厂函数、Messenger 泛型辅助。

---

## 问题清单

### 🔴 严重（影响生成代码正确性）

#### G1. 方法过滤逻辑脆弱且矛盾
- **位置**：`genElementIDInternal` 行 200-206、`genAtomIDInternal` 行 296-302、`genImplement` 行 352-363/373-381/433-448/452-461
- **问题**：6 处几乎相同的遍历+前缀过滤逻辑，条件相互矛盾且重复：
  - `genElementIDInternal` 行 205-206：`ElementSpawn` 的 skip 后又 check `Spawn`（永不执行）
  - `genImplement` 行 354：`|| methodName == "Element"` 是多余防御
- **影响**：改过滤条件需同步改 6 处，极易遗漏；前缀拼错静默跳过无报错

#### G2. genAtomIDInternal 行 509 错误 TrimPrefix
- **位置**：`atomos.go:509`
- **代码**：`atomFnName := strings.TrimPrefix(methodName, "Element")`
- **问题**：Atom 方法已在前面的 `HasPrefix("Element") { continue }` 过滤，这行 trim 永远是 no-op。但若 Atom 方法误叫 `ElementXxx`，名字会被截断
- **影响**：生成的方法名与 proto 不一致（隐患）

#### G3. 前缀约定无 proto 级校验
- **问题**：Element/Atom 区分完全靠方法名 `Element` 前缀，proto 层面无法约束
- **影响**：拼错前缀（如小写 `elementSayHello`）静默分类错误
- **根本方案**：用 proto custom option 标注（见优化建议 G5）

### 🟡 中等（设计/可维护性）

#### G4. CODE JUMPER hack
- **位置**：行 229、239、314、324
- **代码**：`/* CODE JUMPER 代码跳转 */ _ = func() { _ = xxxElementValue.SayHello }`
- **问题**：为 IDE 跳转生成的无用闭包，污染生成代码，依赖未导出变量

#### G5. Messenger 泛型类型参数冗余
- **位置**：`dev_messenger.go:8`
- **代码**：`Messenger[E ID, A ID, AT Atomos, IN, OUT]`
- **问题**：5 个类型参数，Element 方法不用 A，Atom 方法不用 E，增加理解成本

#### G6. 中英文注释混用
- **位置**：多处 `g.P("需要实现的接口")` 等
- **问题**：原样写进生成代码，影响可读性

### 🟢 轻微

#### G7. noExport 过于简陋
- 首字母缩写名（如 `HTTPServer`）→ `hTTPServer`，不符合 Go 惯例

#### G8. genUnstableServerInterfaces 死参数
- `main.go:14` 定义了 flag，`atomos.go` 从不读取

#### G9. Spawn 签名约定不直观
- Element Spawn 的 data 在 output 类型，Atom Spawn 的 arg 在 input 类型——约定隐晦

---

## 优化计划（按可行性排序）

| # | 改进 | 难度 | 优先级 |
|---|---|---|---|
| O1 | 抽取公共方法过滤 helper，消除 6 处重复 | 低 | 高 |
| O2 | 修复 G2 行 509 错误 TrimPrefix | 低 | 高 |
| O3 | 删除 G8 死参数 | 低 | 高 |
| O4 | 中英文注释统一为英文（G6） | 低 | 中 |
| O5 | 用 proto custom option 替代前缀约定（G3） | ✅ 已完成 | 中 |
| O6 | 去掉 CODE JUMPER hack（G4） | 中 | 低 |
| O7 | 精简 Messenger 类型参数（G5） | 高 | 低 |
