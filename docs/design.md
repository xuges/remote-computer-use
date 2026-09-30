# remote-computer-use v1 设计

## 定位

提供两个 Go CLI：`remote-computer-ctl` 管理连接，`remote-computer-use` 提供统一截图和键鼠操作。使用 VNC/RDP 解码到内存并直接发送输入，不需要显示窗口、Xvfb、SSH 或远端执行助手。与 `remote-shell` 独立互补：本项目操作 GUI，remote-shell 执行远端命令和传输文件。

本机实现覆盖 Linux amd64/arm64、macOS arm64、Windows amd64；具体测试层级见跨平台文档。远端需要已启用 VNC/RDP 服务。静默指本机无窗口，远端用户仍可能看到鼠标、输入和应用变化。服务端支持独立会话时使用独立会话，否则使用现有桌面；本工具不能阻止远端物理键鼠输入。

## 架构

每个连接使用一个后台进程，复用 ctl 二进制的内部运行模式。CLI 通过带独立令牌的 loopback HTTP 请求操作，详见 [本机 HTTP 接口](http-ipc.md)。同一连接内输入动作串行，不同连接并行。协议事件循环持续解码，截图读取一致的完整内存快照，由 Go 编码 PNG。

Go 使用 github.com/goexlib/cgo 动态加载小型 C 适配库，Go 二进制使用 CGO_ENABLED=0 构建。VNC 采用 libgvnc 核心，RDP 采用 FreeRDP 3 软件解码。C 适配层封装结构体、线程、回调和内存所有权，原生回调不进入 Go。库通过版本化 ABI 提供连接状态、画面快照和输入；不依赖 X11 客户端或显示服务。

## 发行与安装

发行版使用 `embedded` build tag，通过 `go:embed` 将适配库及非系统的传递依赖压缩归档编入 ctl；use 不调用后端，链接时不包含这份资源。构建时按 ELF、Mach-O、PE 分别重定位并验证依赖闭包，使其不再指向开发机器。系统库与加载器由宿主提供；Linux 基线为 glibc 2.35，macOS 当前依赖要求 15.0，Windows 使用 UCRT64 DLL。

首次使用在当前用户私有缓存中释放，按资源清单哈希隔离版本，逐文件校验 SHA-256，拒绝路径穿越、符号链接和不安全权限，通过临时目录原子发布处理并发启动。运行期间不下载依赖，不删除旧缓存。显式 `REMOTE_COMPUTER_LIB_DIR` 优先，保留开发和替换动态库的能力。

离线安装器先自检运行库，再安装程序与可选 Skill，生成不含真实凭据的示例配置且不覆盖已有配置。`doctor` 独立于配置和远端连接，检查库加载及 ABI；不暴露底层错误原文。升级保留旧版本与活动 worker，重连后生效。发行包附第三方许可和来源清单。实现与排障详见 [安装文档](installation.md)。

验证分三层：缓存与并发单元测试；安装、配置保留、冲突和只有 glibc 的隔离根目录测试；使用发行版 CLI 完成真实 VNC/RDP 回环测试。四个平台的测试层级分别记录，后续补齐原生 ARM Linux 与公开分发验收。

## 公共接口

```sh
remote-computer-ctl list --json
remote-computer-ctl status mac --json
remote-computer-ctl connect mac
remote-computer-ctl disconnect mac
remote-computer-ctl connect --all
remote-computer-ctl disconnect --all

remote-computer-use --conn mac screenshot --output screen.png
remote-computer-use --conn mac screenshot --output - > screen.png
remote-computer-use --conn mac move --x 300 --y 200
remote-computer-use --conn mac click --x 300 --y 200 --button left --count 2
remote-computer-use --conn mac scroll --dy 3
remote-computer-use --conn mac drag --from 100,200 --to 500,300 --duration 500ms
remote-computer-use --conn mac key CMD+L
remote-computer-use --conn windows type --text Hello
remote-computer-use --conn windows type --stdin
```

所有桌面操作显式指定连接，不自动连接。截图必须指定输出文件或 `-`，原始尺寸 PNG，不额外合成光标。PNG stdout 不能混入 JSON 或日志。坐标以截图左上角为原点，越界报错；滚轮 dy 正值向下，dx 正值向右，单位为刻度。组合键顺序按下、逆序释放。文本与快捷键分开，使用 RDP Unicode / VNC keysym；显式 clipboard 方法仅在后端报告支持时允许，不静默降级。文本输入内容不记入日志。

Apple ARD 服务会丢弃非 BMP 的完整码点 keysym。VNC 后端根据服务端提供的 ARD 认证类型识别兼容性路径，将这些字符编码为两个带 Unicode 标记的 UTF-16 代理单元，各自按下并释放。两个单元放在同一个原生队列事件内，避免取消请求在代理对中间截断；传输失败仍返回错误。普通 VNC 继续使用完整码点，不新增用户配置或剪贴板依赖。

操作支持 --json、--timeout；坐标动作支持 --geometry-version，拒绝来自旧尺寸截图的操作。返回成功表示事件已发送，不等于远端应用操作成功，AI 应再次截图确认。拖拽与组合键自动释放本次新按下的输入。

### 跨命令保持输入

针对拖动中插入 Esc 等操作，新增 `pointer-down --x X --y Y [--button left|middle|right]`、`pointer-up [--button ...]`、`key-down KEY`、`key-up KEY` 和 `release-input`。每个 key down/up 只接受一个键；组合修饰键可依次按下。`move` 使用当前按钮掩码，`key` 跳过已经显式保持的键，只逆序释放本次新按下的键。保持鼠标时拒绝完整 click/drag/scroll，保持键时拒绝 type，以免现有自动释放或 Unicode 路径破坏保持状态。

状态由 worker 中的 controller 持有，通过既有 action gate 串行访问；原生 ABI 不变。多条 CLI 命令之间允许截图观察，但不提供调用者隔离或序列事务。成功输入更新 30 秒空闲期限，截图/status 不续期。worker 每 250ms 尝试获取空闲 gate，检查超时、连接状态和尺寸变化；正在执行的动作由自身取消清理负责。清理使用独立的 1 秒上下文，失败的释放保留在跟踪状态中供后续重试；release-input 绕过尺寸/取消检查。正常断开先停止请求并清理输入，再关闭后端。SIGKILL、服务端断开等情况下不承诺释放送达。

后台操作失败清理整个连接的跟踪输入；命令行校验失败尚未到达后台，只能通过 finally/trap 中的 release-input 或空闲期限收尾。升级 CLI 后须重连已有会话以更新 worker。暂不增加 JSON 序列 DSL，先让单一低层操作可以跨命令组合。

## 配置与安全信息

配置默认位于 ~/.remote-computer-use/config.toml，采用 version = 1 和 [connections.NAME]。字段包括 protocol、host、port、username、password/password_env、domain、RDP width/height、可选证书指纹。连接名是非敏感别名，限制为字母、数字、下划线和连字符。

配置文件权限 0600，运行目录 0700。运行目录使用 XDG_RUNTIME_DIR/remote-computer-use 或 /tmp/remote-computer-use-UID。配置与运行目录支持显式覆盖，便于隔离测试。

PrivateConnectionConfig 与 PublicConnectionInfo 必须分离。公开输出只包括别名、协议、状态、尺寸、尺寸版本、帧序号、支持能力及安全错误码。不输出地址、端口、帐号、密码、令牌、证书材料、配置原文。凭据经私有启动管道传递，不放入子进程 argv。底层日志被拦截，不透传原始库错误，不提供显示秘密的调试开关。

TLS 默认验证服务端证书；当前固定指纹功能仅用于 RDP，替代 CA / 主机名验证。VNC 选择 VeNCrypt X509，配置指纹暂返回 UNSUPPORTED。传统无 TLS VNC 需要显式 allow_unencrypted = true。若服务端同时提供传统认证，但 VeNCrypt 没有受支持的 X509 子类型，只在此开关开启后重连尝试一次传统认证；证书或密码失败不触发降级。服务端返回 RFB 3.3 安全类型 0 时按连接拒绝处理，避免把临时限流误报为协议不支持。认证成功不表示桌面已解锁。

为兼容常见自签名 RDP 部署，支持显式 `tls_verify = false`：保持 TLS 加密和原有帐号认证，跳过服务端证书身份校验，不降级为 Standard RDP Security。省略时为 true；false 与 certificate_sha256 互斥，VNC 不接受此字段。该策略只属于私有配置，不加入公开状态或错误输出。原生 ABI 升级为版本 2，CLI 和适配库必须一起升级，旧库会被明确拒绝加载。

## 生命周期

状态为 stopped、connecting、ready、disconnecting、failed。connect 等待认证和首个完整画面，默认 30 秒。重复 connect 健康连接、disconnect 已停止连接是幂等操作。disconnect 等待 worker 生命周期锁释放，确保原生后端清理完成后才允许紧接着重连。配置变更在重新连接时生效。

connect --all 连接当前配置，最多并发 4 个，每项独立返回结果，部分失败不回滚成功连接。disconnect --all 清理当前用户运行的所有连接，包括已从配置删除的连接。任一失败整体退出非零。

同连接操作串行，单动作默认超时 10 秒，可覆盖。断网停止接收输入，不重放动作。取消、超时、失败时尽力释放本次操作按下的键和按钮。尺寸变化使旧画面无效，等待新完整画面后恢复输入。截图只表示最新接收到的完整画面，不是应用完成屏障。断开不主动注销远端用户。

## 实施阶段与验证

1. 设计文档和仓库约定；独立提交。
2. Go CLI、配置校验、私有 IPC、进程生命周期、脱敏查询、批量操作和测试。
3. 原生 ABI、libgvnc 后台截图和输入；真实回环 VNC 集成测试。
4. FreeRDP 3 后端、共享输入语义、异常和内存所有权验证。
5. 构建打包、使用文档；CLI 稳定后适配 Codex/OpenCode/Claude Code Skill。

验收必须覆盖无 DISPLAY/WAYLAND_DISPLAY、截图颜色与尺寸、增量更新、坐标边界、组合键释放、双击/拖拽/滚轮、Unicode 支持范围、多连接隔离、批量部分失败、错误认证、证书失败、断网、取消、进程崩溃和秘密泄露检查。不能把 mock 测试或 Linux 回环测试当作 macOS/Windows 实机验证；未具备实机时明确记录兼容性待验证。

提交使用 Conventional Commits。每个阶段运行相应测试，保持提交可审阅，不提交配置凭据或构建产物。

## 当前进度

Windows x64、Linux x64/arm64、macOS arm64 的平台层、HTTP/权限、动态库分发、证书信任及当前验收结果见 [本机跨平台支持设计](cross-platform.md)。

阶段 1–4 已实现并完成 Go 检查和真实 VNC/RDP 回环验证。阶段 5 已提供 Linux amd64 自包含发行包、离线安装器、自检、共用 Skill 和手动构建工作流；尚未在线发布，也未完成各 Agent 客户端联调。实际覆盖和未完成项见 [验证记录](validation.md)。

已完成配置中的 Linux VNC、macOS 屏幕共享和 Windows RDP 实机基础矩阵，并修复 Mac 非 BMP 文本丢失。Windows 双轴滚动按两次单轴命令缓解，暂不优先修改协议实现。后续处理 ARD 水平滚动，覆盖 Linux xrdp、证书与会话策略互操作，继续完善本机平台验收，后续再扩展剪贴板和可选显示。维持两个公开 CLI 的职责边界，不引入与 remote-shell 重叠的任意 shell 执行接口。
