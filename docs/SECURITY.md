# AtomOS 安全审计与加固指南

状态：已实施（2026-10-28）

本文记录本轮安全审计确认的问题、修复行为和升级要求。安全测试位于
`cluster_security_test.go`、`debug_server_test.go` 和应用文件测试套件中。

## 1. 威胁模型

集群 gRPC 端口必须被视为高权限管理面与业务面入口。仅“位于内网”不构成身份认证；
任何能够连接该端口的进程都可能尝试创建 Atom、调用业务方法或停止节点。

本框架区分两类部署：

- **生产/安全模式**：所有集群 RPC 使用双向 TLS（mTLS），客户端证书包含唯一
  Atomos 节点身份；远程节点管理另有管理员名单。
- **显式不安全模式**：仅供本机开发和测试。TCP 只监听 `127.0.0.1`，远程节点管理
  始终禁用。自定义内存传输用于 DST 和单元测试。

etcd 是集群路由、租约和生命周期记录的信任根，必须独立启用其认证、授权和传输加密。
框架的 mTLS 不替代 etcd 自身安全。

## 2. 已修复问题

| 级别 | 问题 | 修复结果 |
| --- | --- | --- |
| 严重 | 无凭据客户端可创建 Atom、调用业务方法并执行 `TryKilling` | 安全模式强制 mTLS；无证书、明文连接全部拒绝 |
| 高 | 服务端信任请求中的 `CallerId`，可伪造授权身份 | `CallerId` 的 Cosmos/Node 必须与证书 URI SAN 一致 |
| 高 | 任意集群节点都可能执行远程关停和排空 | `TryKilling`、`DrainNode` 仅允许 `admin-nodes`；不安全模式完全禁用 |
| 高（本地前提） | 固定 `run/test` 跟随符号链接并截断目标 | 改为 `os.CreateTemp`；日志和 PID 文件使用防符号链接打开 |
| 中 | 非回环 Debug Bearer token 通过明文 HTTP 传输 | 非回环 Debug 同时强制 HTTPS 和 Bearer token |
| 中 | RPC 日志和 panic 响应可能泄露业务参数 | 日志只保留目标/方法；客户端 panic 错误不再携带请求或 panic 内容 |
| 中低 | 生命周期 RPC 丢弃取消和截止期 | spawn/halt/takeover 传递原始 RPC context |
| 中低 | RPC 缺少明确资源边界 | 请求最大 4 MiB，每连接最多 128 个并发流 |

原攻击验证使用独立无凭据 gRPC 客户端，曾成功完成“创建 Atom → 调用业务方法 →
关闭节点”。回归测试现在要求同一类攻击失败，同时验证合法 mTLS 请求仍然成功。

## 3. 集群证书与节点身份

### 3.1 身份格式

每个节点的客户端证书必须且只能包含一个 Atomos URI SAN：

```text
atomos://<cosmos>/<node>
```

例如：

```text
atomos://shop/nodeA
```

证书仍需具有 Client Auth EKU，并由 `client-ca-path` 指定的 CA 签名。服务端证书
必须具有 Server Auth EKU 和覆盖节点地址的 DNS/IP SAN。框架拒绝：

- 没有 Atomos URI SAN 的客户端证书；
- 含多个 Atomos 身份的证书；
- 证书身份与本地 `cosmos`/`node` 不一致的配置；
- 请求 `CallerId` 与已认证证书身份不一致的调用；
- 安全模式下的 `insecure-skip-verify`。

旧版从 etcd 加载的共享客户端证书也执行相同身份校验。没有唯一节点 URI SAN 的
共享证书不能再用于安全集群；推荐改为每节点独立的文件配置。

### 3.2 YAML 示例

```yaml
cosmos: shop
node: nodeA
enable-cluster:
  enable: true
  etcd-endpoints: [127.0.0.1:2379]
  optional-ports: [23107]
  admin-nodes: [opsNode]
  enable-cert:
    cert-path: /etc/atomos/nodeA-server-chain.crt
    key-path: /etc/atomos/nodeA-server.key
    client-cert-path: /etc/atomos/nodeA-client.crt
    client-key-path: /etc/atomos/nodeA-client.key
    client-ca-path: /etc/atomos/client-ca.crt
```

私钥文件应只允许运行账户读取。每个节点使用自己的客户端证书和私钥；不要把同一
节点私钥复制给整个集群。

### 3.3 管理权限

`admin-nodes` 是节点名列表，权限判断同时依赖证书身份，因此仅伪造节点名无效。
默认列表为空，即所有远程 `TryKilling` 和 `DrainNode` 都被拒绝。

如果运维工具需要远程管理能力，应为其节点签发独立身份，例如 `opsNode`，并仅把该
节点加入目标节点的 `admin-nodes`。普通业务节点不应出现在列表中。

## 4. 本地开发和测试模式

```yaml
enable-cluster:
  enable: true
  etcd-endpoints: [127.0.0.1:2379]
  optional-ports: [23107]
  allow-insecure: true
```

该模式的行为不是恢复旧版裸监听：

- TCP 强制绑定 `127.0.0.1`；
- `TryKilling` 和 `DrainNode` 始终返回 `PermissionDenied`；
- 自定义内存传输仍可用于确定性测试；
- 生产配置不应设置此选项。

这是一个有意的破坏性安全变更：未配置有效 mTLS、且未显式设置
`allow-insecure: true` 的集群将启动失败，而不是静默降级为无认证服务。

## 5. Debug Server 安全

回环监听仍可只使用 HTTP：

```yaml
customize:
  _debug_server: 127.0.0.1:9099
```

非回环监听必须同时配置 HTTPS 证书、私钥和 Bearer token：

```yaml
customize:
  _debug_server: 0.0.0.0:9099
  _debug_server_cert: /etc/atomos/debug.crt
  _debug_server_key: /etc/atomos/debug.key
  _debug_server_token: replace-with-a-long-random-token
```

客户端使用 `https://` 并通过 `Authorization: Bearer ...` 访问。缺少任一安全设置的
非回环 Debug Server 会记录错误并禁用，不影响主进程启动。pprof 和 inventory 仍属于
敏感信息，token 必须独立保管并定期轮换。

## 6. 升级注意事项

1. 为所有节点准备独立客户端证书和 Atomos URI SAN，再启用新版安全模式。
2. 远程运维调用方加入明确的 `admin-nodes`，不要给所有业务节点管理权限。
3. 异步回调协议新增了 `caller_id` 字段；安全模式下，旧节点未提供该字段的回调会
   被拒绝。混合版本集群应先规划证书与版本升级窗口，不要把关闭认证当作兼容方案。
4. 检查依赖固定 `run/test`、宽权限工作目录或明文非回环 Debug 的部署脚本。
5. 保留完整构建、单元测试、集成测试和 Windows 交叉编译作为发布门禁。
