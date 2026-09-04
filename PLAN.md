# sbxctl 方案

## 1. 定位

sbxctl 是连接 sing-box API service 的终端客户端，两种入口：

- `sbxctl` 不带子命令时启动交互式 TUI，用于日常查看和控制。
- 子命令是非交互 CLI，输出稳定、可组合，供用户、脚本和 Agent 调用。

sbxctl 只管理运行时状态。它不读取、生成、修改或重载 sing-box 配置，不启动、停止或托管 sing-box 进程，不实现 Clash HTTP API。

项目名和命令名都是 `sbxctl`。`sbx` 指向 sing-box，`ctl` 表示它是控制和观测现有服务的终端客户端。它是第三方项目，不使用官方图标，不暗示官方维护。

## 2. 调研基线

基线是 2026-09-04 的最新正式版 sing-box 1.14.0。该版本首次正式提供 API service，`GetVersion` 返回 `apiVersion = 4`。

服务端事实，全部在 v1.14.0 源码中核对过：

- TCP 监听。HTTP 用 h2c，HTTPS 用 TLS 并强制加入 `h2` ALPN。
- 请求按 `ProtoMajor == 2` 且 `Content-Type` 为 `application/grpc` 路由到原生 gRPC。gRPC-Web 和 dashboard 走另外的分支，与本项目无关。
- 鉴权是 `authorization: Bearer <secret>` metadata，一元和流式拦截器都校验。secret 为空时不鉴权。
- 远程 API 只注册 attached `StartedService`。attached 实例的 ServiceStatus 恒为 STARTED，没有配置和进程生命周期 RPC。
- 上游 `daemon/client.go` 用 `grpc.NewClient` 加 `insecure.NewCredentials()` 或 `credentials.NewTLS` 直连，认证通过客户端拦截器注入。sbxctl 采用同样方式。

未发版的 main 分支 commit `a556d49`（2026-09-01）把 Clash mode 从 Clash API server 拆成独立的 `clashmode.Manager`。启用 API service 后该 manager 总是存在，不再依赖 `clash_api` 配置块。RPC 签名和 proto 未变。这意味着 Clash mode 是 API service 自带的运行时能力，纳入范围。

### 使用的 RPC

| 能力 | RPC | 说明 |
| --- | --- | --- |
| 版本 | `GetVersion` | 版本号和 apiVersion |
| 服务状态 | `SubscribeServiceStatus` | 远程恒为 STARTED，用作连接心跳 |
| 运行状态 | `SubscribeStatus` | 内存、goroutine、连接数、速率、总量，可指定 interval |
| 启动时间 | `GetStartedAt` | 计算运行时间 |
| 弃用警告 | `GetDeprecatedWarnings` | 在状态里提示 |
| 出站组 | `SubscribeGroups` | 组、当前选择、成员、延迟 |
| 全部出站 | `SubscribeOutbounds` | 所有 outbound 和 endpoint |
| 选择节点 | `SelectOutbound` | 只对 selector 有效 |
| 测速 | `URLTest` | 只触发，结果走 Groups/Outbounds 流 |
| Clash mode | `GetClashModeStatus`、`SubscribeClashMode`、`SetClashMode` | 不可用时返回 NotFound |
| 连接 | `SubscribeConnections` | 事件流，可指定 interval |
| 关闭连接 | `CloseConnection`、`CloseAllConnections` | |
| 日志 | `SubscribeLog`、`GetDefaultLogLevel`、`ClearLogs` | |

### 服务端行为要点

实现时依赖这些行为，端到端验证时逐条复核：

- `SubscribeGroups` 和 `SubscribeOutbounds` 是事件驱动的全量快照，由 URL test 历史更新和服务状态变化触发，最小推送间隔 250ms。客户端整体替换即可。`readGroups` 会跳过没有成员的组。
- `Group.selectable` 只对 selector 为 true。`Group.isExpand` 是官方移动端存在 cache_file 里的折叠状态，sbxctl 忽略。
- `URLTest` 对 urltest 组调用 `CheckOutbounds`，对其他组测试全部成员，对单个 outbound 测一次。单节点测速失败时删除该节点的历史记录，不写错误。所以"记录消失"是失败信号，"urlTestTime 更新"是成功信号。
- `SelectOutbound` 对不存在的组返回 NotFound，对非 selector 返回 InvalidArgument，对不在组内的节点返回 NotFound。
- `SubscribeStatus` 和 `SubscribeConnections` 的 `interval` 是纳秒整数，非正数时服务端用 1 秒。
- `Status.trafficAvailable` 为 false 时没有流量和连接数。`SubscribeConnections` 在实例没有连接跟踪时返回 Unimplemented。
- `SubscribeConnections` 第一帧 `reset = true`，包含活动连接和服务端保留的已关闭连接，后者带 `closedAt`。之后是 NEW、UPDATE、CLOSED 事件，UPDATE 携带流量增量。客户端自己维护连接表。
- `SubscribeLog` 第一帧 `reset = true`，包含最多 3000 行历史。`ClearLogs` 也会推一帧 `reset`。消息由平台 formatter 生成，包含 ANSI 颜色码和相对时间戳。服务端不按级别过滤，`GetDefaultLogLevel` 返回实例配置的级别供客户端默认过滤。
- Clash mode 相关 RPC 在实例没有 mode manager 时返回 NotFound。
- 服务未就绪时一元 RPC 返回 `os.ErrInvalid`，到客户端是 Unknown code 加 "invalid argument" 文本。远程 attached 实例不会出现，但错误映射要覆盖。
- 服务端接受 `accept-language` metadata，只影响弃用警告和 Tailscale 文本。sbxctl 不发送。

## 3. 功能边界

### 包含

- 保存和切换多个 API 连接目标，也可用临时 URL 和 secret 直连。
- HTTP 和 HTTPS，HTTPS 支持私有 CA、自定义 server name 和跳过证书校验。
- 服务版本、API 版本、连接状态、运行时间、速率、总流量、内存、goroutine、连接数、弃用警告。
- 出站组、组成员、当前选择、类型、延迟、最后测试时间。
- 全部 outbound 和 endpoint。
- 搜索和排序组与节点。
- 切换 selector 节点。
- 触发单节点和整组测速，CLI 可等待结果。
- 查看和切换 Clash mode。
- 查看、筛选、排序和关闭连接。
- 查看、筛选、暂停和清空日志。
- 所有读取和控制能力都有非交互 CLI，快照和流式命令都有机器可读输出。

### 不包含

- sing-box 配置的读取、编辑、校验、写入、重载。
- 节点、订阅、Provider、规则集、DNS 管理。
- sing-box 进程的启动、停止、重启、升级、守护。
- Clash HTTP API 及任何兼容层。
- Network Quality、STUN、Tailscale、Taildrop、OpenVPN、OpenConnect、USB/IP、Notifications。
- Web UI、托盘、系统代理、桌面通知、移动端。
- 图表、地图、拓扑、主题系统、鼠标驱动的交互契约。

## 4. 连接目标

目标是 sbxctl 自己的连接配置。每个目标包含：

- 唯一名称。
- API URL，scheme 只允许 `http` 或 `https`，path 被忽略。
- 可选 secret。
- 可选 TLS 选项：CA 文件路径、server name、跳过证书校验。

配置文件放在 `os.UserConfigDir()/sbxctl/config.toml`，Linux 下遵循 XDG。文件权限 0600，目录 0700。文件记录目标列表和当前目标名。

连接参数按优先级解析，高优先级整体覆盖低优先级：

1. `--url`、`--secret` 等命令行参数。
2. `--target` 或 `SBXCTL_TARGET` 指定的已保存目标。
3. `SBXCTL_URL`、`SBXCTL_SECRET` 环境变量。
4. 配置文件中的当前目标。

`--url` 和 `--target` 同时出现是参数错误。没有任何目标可用时，CLI 报错退出，TUI 打开目标管理界面。

HTTP 正常支持，不显示阻断性安全提示。HTTPS 默认用系统证书池。

## 5. CLI 设计

所有子命令非交互。数据写 stdout，诊断和错误写 stderr。

### 命令

```text
sbxctl                                        启动 TUI
sbxctl target list
sbxctl target show [name]
sbxctl target add <name> <url> [--secret] [--ca <file>] [--server-name] [--insecure]
sbxctl target remove <name>
sbxctl target use <name>

sbxctl status [--watch] [--interval <duration>]
sbxctl groups [--watch]
sbxctl groups show <group> [--watch]
sbxctl outbounds [--watch]
sbxctl select <group> <outbound>
sbxctl test <outbound-or-group> [--wait <duration>]
sbxctl mode [name]

sbxctl connections [--watch] [--state active|closed|all] [--interval <duration>]
sbxctl connections show <id>
sbxctl connections close <id>
sbxctl connections close --all --yes

sbxctl logs [--follow] [--level <level>] [--search <text>]
sbxctl logs clear --yes
```

全局参数：

```text
--target <name>
--url <url>
--secret <secret>
--output table|json|jsonl
--timeout <duration>       一元调用和建连超时，默认 10s
--no-color
```

### 语义

- `status` 输出版本、apiVersion、运行时间、Clash mode、速率、总量、内存、goroutine、连接数和弃用警告。连接跟踪或 mode 不可用时对应字段为空，不是错误。
- `groups`、`outbounds`、`connections` 不带 `--watch` 时取流的第一帧后退出，`--watch` 持续输出。
- `test` 默认只触发并返回。`--wait` 时先订阅 Groups 和 Outbounds 流，再触发，然后等待被测节点的 `urlTestTime` 晚于触发时刻或记录消失，全部到齐或超时后输出每个节点的结果，状态是 `ok`、`failed` 或 `timeout`。完成判定规则在端到端验证阶段按真实行为修正。
- `mode` 不带参数显示当前 mode 和可用列表，带参数切换。mode 名称大小写不敏感。
- `connections` 默认 `--state active`。id 是服务端 UUID。
- `logs` 默认输出服务端缓冲的历史行后退出，`--follow` 持续输出。`--level` 是显示上限，默认取 `GetDefaultLogLevel`，只能过滤不能放大。`--search` 是大小写不敏感的子串匹配。
- `--watch` 和 `--follow` 在终端下原地刷新表格，非终端下逐帧输出。

### 输出契约

- `table` 在终端下是紧凑表格，输出到管道时不带颜色和光标控制符。
- `json` 用于快照。`jsonl` 用于流式输出，每行一个完整对象。`--watch`、`--follow` 与 `json` 组合是参数错误。
- 机器格式只有数据，没有提示、进度和装饰。日志消息在机器格式中剥掉 ANSI。
- 字节数是整数，延迟是整数毫秒，时间是 RFC 3339 字符串。速率是每秒字节整数。
- 字段名和排序稳定。组和节点按上游顺序，连接按创建时间，日志按接收顺序。
- 破坏性操作要求显式 `--yes`。
- 错误信息包含操作、目标和原因。gRPC code 映射：Unavailable 是连接失败，Unauthenticated 是 secret 错误，DeadlineExceeded 是超时，Unimplemented 和 NotFound 是当前实例不支持该能力或对象不存在，其余透传服务端消息。
- 退出码：0 成功，1 远程错误（连接、认证、超时、API 拒绝、版本不兼容），2 参数或本地配置错误。

TUI 和 CLI 共用同一套领域模型和操作，不维护两套行为。

## 6. TUI 设计

### 目标

精致、干净、克制。信息密度高但不拥挤。键盘操作对标 nvim 和 yazi：常用动作单键可达，动作可预期，不需要看提示也能猜对。

### 布局

```text
┌ 顶栏 ──────────────────────────────────────────────────────┐
│ home ● 1.14.0  up 3d 4h  rule  ↑ 1.2 MB/s ↓ 8.4 MB/s  42 conn │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  Proxies  Connections  Logs           (当前工作区高亮)      │
│                                                            │
│  工作区内容                                                │
│                                                            │
├────────────────────────────────────────────────────────────┤
│ j/k move  enter select  t test  T test group  / filter  ? help │
└────────────────────────────────────────────────────────────┘
```

- 顶栏一行：目标名、连接状态点、服务版本、运行时间、Clash mode、上下行速率、活动连接数。连接跟踪不可用时省略流量和连接数。
- 工作区标签一行，三个固定工作区：Proxies、Connections、Logs。
- 底栏一行，只显示当前上下文最常用的键。完整键位在 `?` 帮助层。
- 筛选、确认和消息在底栏位置临时替换显示，不弹独立窗口。
- 帮助、连接详情和目标选择用居中浮层，浮层是唯一使用边框的地方。

### 键位

全局：

| 键 | 动作 |
| --- | --- |
| `1` `2` `3` | 切换到 Proxies、Connections、Logs |
| `Tab` `Shift+Tab` | 下一个、上一个工作区 |
| `j` `k` `↓` `↑` | 上下移动 |
| `gg` `G` | 顶部、底部 |
| `Ctrl+d` `Ctrl+u` | 半页 |
| `Ctrl+f` `Ctrl+b` | 整页 |
| `/` | 筛选，实时过滤，`Enter` 保留，`Esc` 清除 |
| `s` | 循环排序键 |
| `S` | 反转排序方向 |
| `m` | 循环 Clash mode，不可用时不显示 |
| `Ctrl+t` | 目标选择浮层 |
| `R` | 手动重连 |
| `?` | 帮助 |
| `Esc` | 关闭浮层、清除筛选、取消确认 |
| `q` | 退出，浮层打开时先关闭浮层 |
| `Ctrl+c` | 强制退出 |

Proxies：

| 键 | 动作 |
| --- | --- |
| `h` `l` `←` `→` | 左右栏切换焦点 |
| `Enter` | 左栏进入组；右栏在 selector 中选择节点 |
| `t` | 测试焦点节点；焦点在左栏时测试焦点组 |
| `T` | 测试当前组全部成员 |

Connections：

| 键 | 动作 |
| --- | --- |
| `Enter` | 打开、关闭详情 |
| `a` | 循环 active、closed、all |
| `x` | 关闭焦点连接 |
| `X` | 关闭全部连接，需确认 |

Logs：

| 键 | 动作 |
| --- | --- |
| `Space` | 暂停、恢复自动滚动 |
| `G` | 跳到最新并恢复自动滚动 |
| `L` | 循环显示级别 |
| `c` | 清空日志，需确认 |

确认提示在底栏显示，`y` 确认，其他键取消。

### Proxies

宽终端左右双栏，左栏占三分之一。

- 左栏列出所有组，最后是固定项 `All outbounds`。组行显示名称、类型、当前选择、成员数。
- 右栏显示当前组成员或全部出站。节点行显示选中标记、名称、类型、延迟、最后测试时间。
- selector 允许选择，选择后立即调用 `SelectOutbound`，以流回传的 `selected` 为准更新选中标记。其他组只读。
- 排序键：上游顺序、名称、延迟。延迟排序时无记录的节点排最后。
- 筛选只作用于当前焦点栏。
- 触发测速后，被测节点显示 `testing` 直到流更新或 10 秒后放弃。记录消失显示 `failed`。不伪造进度。
- 窄终端只显示一栏，`Enter` 进组，`Esc` 或 `h` 返回组列表。

### Connections

- 默认显示活动连接。已关闭连接由客户端保留，上限 1000 条，超出丢弃最旧的。
- 列：入站、来源、目标（domain 优先，否则地址）、规则、出站链、上行、下行、时长。窄终端压缩为来源、目标、出站、流量。
- 排序键：创建时间、实时速率、累计流量。
- 筛选匹配目标、domain、来源、入站、出站、规则、进程路径。
- 详情浮层显示全部字段：id、network、ipVersion、protocol、user、fromOutbound、chainList、processInfo、创建和关闭时间、流量。
- 只渲染可视窗口内的行。

### Logs

- 客户端缓冲上限 5000 行，覆盖服务端 3000 行历史。
- 默认显示级别取 `GetDefaultLogLevel`，`L` 只能在该级别及更严格之间循环。
- 保留服务端 ANSI 颜色，统一到当前终端色彩能力。
- 暂停时新日志继续入缓冲，底栏显示暂停标记和未读计数。
- 筛选是大小写不敏感的子串匹配，匹配部分高亮。

### 视觉

- 一个主强调色，加成功、警告、错误三种语义色。启动时请求终端背景色，按明暗选择两套颜色值，不提供主题配置。
- 层级靠前景明暗、字重和留白表达。主体内容没有边框，只有浮层有边框。
- 焦点行用强调色背景。当前选择的节点用强调色标记和加粗。危险操作确认用错误色。
- 延迟用低饱和语义色分档，数值始终显示。无记录显示 `-`。
- 不用图标字体、渐变、动画、ASCII Logo。
- 终端无色彩时靠文本标记辨认状态。
- 80 列 24 行可完整操作，宽终端有效利用双栏。

### 连接状态

- 建连中和重连中顶栏显示黄色状态点和重试次数，内容区保留上次数据并标注过期。
- 认证失败不自动重连，顶栏显示错误，提示 `Ctrl+t` 换目标或 `R` 重试。
- 切换目标时清空所有工作区数据。

## 7. 技术方案

### 技术栈

| 组件 | 版本 |
| --- | --- |
| Go | 1.27 |
| bubbletea/v2 | v2.0.9 |
| lipgloss/v2 | v2.0.6 |
| bubbles/v2 | v2.2.1，用 viewport、textinput、key、help |
| cobra | 最新 |
| grpc-go | v1.83.2 |
| protobuf-go | 最新 |
| BurntSushi/toml 或 pelletier/go-toml/v2 | 配置文件 |

### API 接入

- 仓库保留上游 `daemon/started_service.proto` 的逐字节副本，`api/UPSTREAM` 记录 tag、commit 和 sha256。
- 用 `protoc-gen-go` 和 `protoc-gen-go-grpc` 生成到 `internal/daemon`，通过 `--go_opt=M` 和 `--go-grpc_opt=M` 覆盖上游 `go_package`，proto 文件本身不改。生成命令写在 `go generate` 指令里，生成代码提交到仓库。
- 生成代码包含 Tailscale、USB 等未使用消息，是死代码，不裁剪。
- 不导入 `github.com/sagernet/sing-box` 任何包。`daemon` 包会连带编译 sing-box 核心。

连接建立流程：

1. 解析 URL，按 scheme 选 `insecure.NewCredentials()` 或 `credentials.NewTLS`，缺省端口 80 或 443。
2. `grpc.NewClient`，一元和流式拦截器注入 `authorization` metadata。
3. 调用 `GetVersion`，`apiVersion < 4` 时报版本不兼容，更高版本继续使用已知 RPC。
4. 启动 ServiceStatus、Status、Groups 常驻流，其余流按需启动。

protobuf 对新增字段前向兼容。新 API 只在 sbxctl 明确使用时才同步 proto 并更新领域类型。

### 结构

```text
CLI / TUI
   |
internal/sbx      session、领域类型、转换、操作
   |
internal/daemon   生成的 gRPC client
   |
sing-box API service
```

`internal/sbx` 是唯一接触生成类型的包。领域类型是普通 Go struct，带 JSON tag，CLI 直接序列化，TUI 直接渲染。

Session：

- 一个活动目标对应一个 Session，拥有 `grpc.ClientConn`、根 context 和所有流 goroutine。
- 常驻流：ServiceStatus、Status、Groups、ClashMode。ClashMode 返回 NotFound 时标记不可用并停止重试。
- 按工作区启停：Outbounds、Connections、Logs。
- 每条流独立重连，退避 1s 起翻倍到 30s 上限。Unauthenticated 不重连。
- Session 通过一个事件 channel 向 TUI 推送更新，TUI 用一个 Cmd 循环读取。CLI 直接调用 Session 方法取快照或迭代流。
- 切换目标时取消旧 Session 的 context，等待 goroutine 退出后再建新的。

连接表、日志缓冲和 URL test 追踪状态放在 `internal/sbx`，TUI 和 CLI 共用。

## 8. 许可证与上游边界

sbxctl 采用 `GPL-3.0-or-later`。项目包含 sing-box 的 proto 和生成代码，公开源码或分发二进制时整个组合按 GPLv3 或更高版本提供，并提供可构建的对应源码。

- 只同步 `daemon/started_service.proto` 和生成的 Go 文件。
- 保留上游版权、GPL 条款和额外名称条款，记录每次同步对应的 sing-box tag。
- 项目名独立，说明文字写明是第三方客户端。
- sing-box-dashboard、官方客户端和 zashboard 只用于行为调研，不复制 UI 或实现。

仅在个人环境制作和运行副本不触发公开源码义务。以上是工程处理方案，不是法律意见。

## 9. 工程结构

```text
cmd/sbxctl/main.go                  入口
api/daemon/started_service.proto    上游协议副本
api/UPSTREAM                        上游 tag、commit、sha256
internal/daemon/                    生成代码
internal/sbx/                       连接、session、领域类型、转换、操作、错误映射
internal/config/                    目标配置读写
internal/cli/                       Cobra 命令树、输出格式
internal/ui/                        Bubble Tea 模型、视图、样式、键位
```

不引入插件系统、依赖注入框架、事件总线、主题配置。

## 10. 实施顺序

1. Go module、入口、`internal/config`、连接参数解析、`target` 子命令。
2. 同步 proto 并生成 client，`internal/sbx` 完成建连、认证、TLS、版本检查、错误映射，`status` 命令跑通。
3. `internal/sbx` 完成 Groups、Outbounds、Status、ClashMode、Connections、Logs 流，连接表和日志缓冲，`select`、`test`、`mode`、`connections`、`logs` 命令。
4. CLI 输出格式、`--watch`、`--follow`、`--wait`、退出码。
5. TUI 骨架：顶栏、工作区切换、底栏、帮助、目标选择、确认提示、明暗适配。
6. TUI Proxies、Connections、Logs 工作区，双栏和窄终端布局。
7. 断线重连、状态过期标注、终端降级。
8. 对真实 sing-box 1.14.0 实例端到端验证，按真实行为修正测速完成判定和连接事件处理，检查机器输出稳定性。

## 11. 验收标准

- 不导入 sing-box Go module 即可构建。
- HTTP 和 HTTPS 都能连接本地与远程实例，私有 CA 和跳过校验可用。
- secret 对一元和流式 RPC 都生效，错误 secret 得到明确的认证失败。
- `apiVersion < 4` 得到明确的版本不兼容错误。
- selector 切换、单节点测速、组测速、mode 切换可从 TUI 和 CLI 执行。
- `test --wait` 输出每个节点的 ok、failed 或 timeout。
- 连接跟踪和 Clash mode 不可用的实例上，TUI 和 CLI 都能正常使用其余功能。
- TUI 持续接收状态、组、连接、日志更新，切换目标后没有旧数据。
- 断线后自动重连并恢复所有流，认证失败不重连。
- 80 列 24 行可完整操作，宽终端双栏，明暗背景下都可读。
- 所有键位符合第 6 节的表，`?` 帮助内容与实际键位一致。
- `json` 和 `jsonl` 输出不含 ANSI、提示和进度文本，字段和排序稳定。
- 破坏性 CLI 操作都要求 `--yes`，TUI 都要求确认。
- 退出码符合第 5 节的定义。
- 上游 proto、生成代码、许可证和同步记录完整。

## 12. 调研来源

- [sing-box 1.14.0 release](https://github.com/SagerNet/sing-box/releases/tag/v1.14.0)
- [sing-box API service 配置](https://sing-box.sagernet.org/configuration/service/api/)
- [StartedService proto](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/started_service.proto)
- [StartedService 实现](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/started_service.go)
- [gRPC server 与认证拦截器](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/server.go)
- [上游 remote client](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/client.go)
- [attached service](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/attached_service.go)
- [API service TCP、TLS、h2c](https://github.com/SagerNet/sing-box/blob/v1.14.0/service/api/server.go)
- [gRPC 与 gRPC-Web 路由](https://github.com/SagerNet/sing-box/blob/v1.14.0/service/api/web_bridge.go)
- [日志平台 formatter](https://github.com/SagerNet/sing-box/blob/v1.14.0/log/observable.go)
- [Clash mode 独立化 commit a556d49](https://github.com/SagerNet/sing-box/commit/a556d49491c23db6117aea241a778e2c3a9e498f)
- [官方 dashboard](https://github.com/SagerNet/sing-box-dashboard)
- [Bubble Tea v2](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss v2 明暗适配](https://github.com/charmbracelet/lipgloss#readme)
- [Bubbles v2](https://github.com/charmbracelet/bubbles)
- [sing-box LICENSE](https://github.com/SagerNet/sing-box/blob/v1.14.0/LICENSE)
- [GNU GPLv3](https://www.gnu.org/licenses/gpl-3.0.html)
- [GNU GPL FAQ：私下修改与使用](https://www.gnu.org/licenses/gpl-faq.html#GPLRequireSourcePostedPublic)
- [GNU GPL FAQ：GPL library linking](https://www.gnu.org/licenses/gpl-faq.html#IfLibraryIsGPL)
