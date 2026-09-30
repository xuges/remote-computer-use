# 验证记录

## 自动化验证

环境：Ubuntu 22.04 x86_64、Go 1.25.8、libgvnc 1.3.0、FreeRDP 3.25.0。

| 层次 | 覆盖内容 |
| --- | --- |
| Go 单元测试与 race | 配置权限/未知字段/凭据互斥、私有错误边界、IPC 分帧与大数据、取消释放、组合键部分失败释放、坐标与尺寸版本、无关命令参数拒绝、断开等待后台清理 |
| Go 静态检查 | `go vet ./...` |
| 构建与安装 | 固定归档校验、无 X11 的 FreeRDP、两种适配库动态加载、临时目录安装 CLI、三个 Agent 的 Skill 安装路径/幂等/覆盖保护与格式校验 |
| 原生输入回归 | ARD 非 BMP 文本使用 UTF-16 代理对、边界码点与中途传输失败；标准 VNC 保持完整码点；ARD 水平滚动在发送任何事件前拒绝，不产生点击 |
| 真实 VNC 回环 | VeNCrypt/传统认证并存、默认拒绝匿名 TLS、显式允许后回退、密码认证、RGB PNG、增量画面、移动/双击/拖拽/双轴滚轮/组合键/文本、真实尺寸变化、旧坐标拒绝、客户端退出释放鼠标 |
| 真实 RDP 回环 | TLS、默认拒绝自签名证书、固定证书指纹与错误指纹拒绝、显式跳过证书校验、NLA 正确/错误密码、RGB PNG、鼠标/键盘/正负双轴滚轮事件 |
| 生命周期 | 重复连接/断开、并行 VNC/RDP 会话、批量部分失败保留健康会话、服务端断开后操作失败、全部断开清理 socket |
| 脱敏与后台运行 | CLI 无 DISPLAY/WAYLAND_DISPLAY、输出不包含测试帐号和密码、公开状态不包含地址与凭据字段 |
| 发行包缓存 | 并发首次解压、重复使用、损坏与不安全权限拒绝、符号链接/路径穿越/错误架构拒绝、显式外部库覆盖 |
| 离线安装 | 路径含空格、重复安装、已有配置保留与新配置 0600、三种 Skill 路径、已有不同 Skill/无关命令不覆盖 |

本次执行：`make test`、`make native-input-test`、`make integration-rdp-test` 均通过。构建使用 `build/native-headless` 下的 CMake 配置。测试服务只绑定回环地址，使用临时配置和测试凭据，退出后清理。

FreeRDP shadow 的水平滚轮映射与 Windows 实机方向不同；回环测试明确检查 shadow 的事件行为，不能由它推断 Windows 的方向。实机另用浏览器页面实际滚动位置验证。

## 自包含发行包验证（2026-09-30）

在 Ubuntu 22.04 amd64 构建，收集 41 个原生库，压缩资源约 22.5 MiB；ctl 约 27 MiB、use 约 3.3 MiB，最终 `.tar.gz` 约 26 MiB。只携带两个 CLI 也可以自检和连接，无需旁置适配库或原构建路径。

`tests/package.py --chroot` 创建仅含宿主 glibc/加载器和发行版 CLI 的临时根目录，以普通 UID 运行 `doctor`，VNC/RDP 均就绪。隔离目录没有 FreeRDP、libgvnc、OpenSSL 系统安装，也没有挂载仓库。该检查验证库加载与 ABI，不替代远端功能测试。

另将 `RCU_BIN_DIR` 指向解压后的发行版，执行 `tests/integration.py --rdp`：真实 VNC 截图与键鼠、认证回退、尺寸变化、取消和生命周期，以及 RDP TLS、指纹、自签名显式跳过、NLA 正确/错误认证、截图和键鼠均通过。测试使用临时桌面，没有操作用户配置的三台主机。

离线安装测试在临时 HOME 下完成，未改动用户的真实配置、PATH 或 Skill。GitHub 手动工作流已编写，尚未在 GitHub 执行；目前产物未发布。其他 Linux 发行版、CA 信任库和企业认证扩展仍需分别验证。

## 三台配置主机实测（2026-09-30）

使用用户私有配置中的 `linux`、`windows`、`mac`，不在记录中保存地址、帐号、密码或桌面截图。Linux 是配置所指向的现有 TigerVNC 桌面服务（位于本机），不是集成测试创建的桌面；Windows 使用 RDP，Mac 使用系统 VNC/ARD。

Linux 按用户明确授权设置 `allow_unencrypted = true`；Windows 保持已授权的 `tls_verify = false`，连接仍使用 TLS。未修改真实帐号、密码或服务端设置。

| 功能 | Linux VNC | Windows RDP | Mac VNC/ARD |
| --- | --- | --- | --- |
| 连接、状态、重复连接 | 通过 | 通过 | 通过 |
| PNG 文件、PNG stdout 解码 | 2560×1369 | 1600×900 | 1650×1050 |
| 指针移动、左/中/右键、双击 | 通过 | 通过 | 通过 |
| 拖拽、取消拖拽后的释放 | 通过 | 通过 | 通过 |
| 垂直正负滚动 | 通过 | 通过 | 通过，单刻度移动较小 |
| 水平正负滚动 | 通过 | 单轴通过 | 不支持，现返回 `UNSUPPORTED` |
| 同命令双轴滚动 | 通过 | Edge 中结果不稳定，待排查 | 不支持，不发送任何轴的输入 |
| 组合键及修饰键释放 | 通过 | 通过 | 通过 |
| ASCII、中文、é | 通过 | 通过 | 通过 |
| 非 BMP 字符 `😀` | 通过 | 通过 | 修复后通过，`A😀B` 完整保留 |
| stdin 文本及换行 | 未单独复测；回环覆盖 | 通过 | 通过 |
| 越界、错误 geometry_version | 正确拒绝 | 正确拒绝 | 正确拒绝 |
| 单连接断开后立即重连 | 通过 | 通过 | 通过 |

还验证了 `list`、三台 `status`、`disconnect --all`、重复断开、断开后的 `NOT_CONNECTED`、`connect --all`、无 DISPLAY/WAYLAND_DISPLAY 的三路并发截图，以及断开 Linux 不影响其他两台。结束时保留三台就绪连接。

测试使用临时文本编辑器文档及 [桌面事件页面](../tests/desktop.html)。页面记录按钮按下/释放、双击、滚轮、实际滚动位置、修饰键与最终文本。Windows 和 Mac 的中文与 emoji 也在原生编辑器中核对过。测试文档未保存，测试标签页已关闭。

### 本次发现与处理

- **VNC 认证选择**：Linux 同时提供 VeNCrypt 和传统密码认证。修复客户端在没有受支持 X509 子类型时直接失败的问题：仅在显式允许传统 VNC 后重连尝试一次，证书或密码错误不触发回退。
- **连接限流**：多次未完成的认证探测触发 TigerVNC 的临时黑名单。停止频繁探测，等待后以配置密码完成一次传统认证；随后普通 CLI 连接、批量重连均通过。把服务端安全类型 0 的拒绝从 `UNSUPPORTED` 改为 `CONNECT_FAILED`，不暴露服务端错误原文。
- **断开竞争**：socket 先于原生连接释放，曾导致立即重连返回 `NOT_READY`。现在等待 worker 生命周期锁释放；加入可复现的单元测试，实机立即重连通过。
- **Mac 水平滚轮**：标准 VNC 按钮 6/7 被 ARD 解释为点击。已在发送前返回 `UNSUPPORTED`，混合 dx/dy 请求也整体拒绝；原生回归测试与 Mac 页面确认不再产生误点击。水平操作可使用截图定位后拖动应用滚动条。
- **Mac emoji**：已修复。原先发送完整 Unicode 码点 keysym 时，`A😀B` 实际为 `AB`；识别到 ARD 后改用 UTF-16 代理对键事件，Safari 和 TextEdit 均能输入 emoji。普通 VNC 保持完整码点，不依赖剪贴板或远端助手。详细复测见下节。
- **Windows 双轴滚动**：单轴正负方向与实际滚动位置验证通过；Edge 中同一命令指定 dx/dy 曾只产生水平变化。暂分成两次单轴调用，该组合场景不能标为通过。
- **焦点与输入法**：点击或启动应用后立即输入可能赶在焦点切换之前；Linux 中文输入法会转换 ASCII。测试期间发生的短文本误输入已撤销或清理，没有保存测试文档。应等待并重新截图确认目标输入框；共享桌面有人同时操作时先协调使用时间。

### 复测方法

在独立浏览器标签页打开 `tests/desktop.html`（可使用本地文件或在地址栏输入其 Base64 `data:text/html` URL）。测试页不联网，也不需要远端常驻助手。先截图确认页面、原始分辨率和焦点，再测试：

1. 在蓝色区域移动、分别点击三键、双击和拖拽，观察页面计数及选择结果。
2. 分别发送正负 dx、dy，截图检查实际 scrollX/scrollY，再单独检查双轴组合。
3. 在文本框输入 `Hello 中文 é A😀B`、换行和 stdin 文本，检查最终字符；使用 `SHIFT+LEFT` 确认选择和修饰键释放。
4. 在空白蓝色区域启动长拖拽并终止本地 CLI，确认释放事件。浏览器可能对选中文本拖拽执行搜索，测试前先清除选择。
5. 验证越界与尺寸版本拒绝、PNG stdout、单个/全部连接生命周期，关闭测试标签页。

这些是固定环境下的功能验收，不等同于所有应用、键盘布局和输入法都兼容。

### Mac 文本修复专项复测（2026-09-30）

在同一台 Mac 的 Safari 临时页面对比编码：完整码点 `0x0101f600` 被丢弃；依次发送 `0x0100d83d`、`0x0100de00` 的按下/释放事件，得到正确的 `U+1F600`。据此只修改 Apple ARD 的非 BMP 分支；两个 UTF-16 单元放在一个原生事件内处理。

使用新增的 [文本比较页面](../tests/text.html)，通过正式 CLI 的 `type --stdin` 输入：

```text
A😀B | 中文 é é | 𠀀𝄞 | 😀😀 | 👍🏽👩‍💻🇨🇳
END
```

Safari 显示 `Exact match: PASS`，实际字符串与预期逐码点一致，覆盖相邻 emoji、CJK 扩展 B、音乐符号、组合重音、肤色、ZWJ、国旗与换行。`U+20000` 在该字体下显示缺字框，但页面读回的码点正确，不能将缺字框等同于输入丢失。TextEdit 新建临时文档以 `type --text` 输入同一组内容，目视核对 emoji、中文与换行；未把 TextEdit 的目视检查当作逐码点比较。

本次执行 `cmake --build build/native-headless --parallel 4`、`make native-input-test`、`make test`、`make integration-test` 均通过。原生回归检查 ARD 与标准 VNC 的 BMP、非 BMP 和边界编码，以及代理对四个发送步骤任一步失败时返回错误；真实 TigerVNC 回环确认普通 VNC 输入路径仍正常。RDP 代码未修改，本次未重跑 Windows 实机测试；双轴需求继续分两次单轴调用。

复测只重连 Mac 以加载新适配库。测试结束关闭临时页面和未保存文档，保留 Mac 就绪连接。

## 历史连接验证

早期 Windows 只接受 Standard RDP Security，启用 TLS 后又因自签名证书被默认校验拒绝。显式配置 `tls_verify = false` 后连接与认证通过。Mac 首次连接直接取得已解锁桌面；VNC 认证和 macOS 登录/解锁是两步，不能由此推断锁屏后不需要解锁。

## 待验证与暂不实现

- 上述 Mac 水平滚动，以及 Windows 双轴组合滚动；Mac 非 BMP 修复尚需更多 macOS 版本和第三方 VNC 服务验证。
- Linux xrdp 实机；VNC VeNCrypt X509 完整证书链/主机名互操作；RDP 企业 CA、域帐号、锁屏和服务器会话策略。
- 更多非拉丁文字、键盘布局和输入法组合。
- 长时间运行、网络抖动、大分辨率负载、原生 sanitizer；Go race detector 不覆盖 C 层。
- 原生进程崩溃恢复、RDP 动态分辨率、物理多显示器专项验收。
- 剪贴板输入、视频显示、音频、文件/设备重定向尚未实现；本机跨平台进展见下节。
- Codex、OpenCode、Claude Code 的完整端到端联调；现有验证只覆盖 Skill 格式与安装流程。

## 本机跨平台与 HTTP 验证（2026-09-30）

本轮沿用已有 SSH 连接上传源码，在 Mac arm64 与 Windows x64 上构建、执行 CLI。安装了 Mac Homebrew 构建依赖、Windows MSYS2 UCRT64 构建依赖；运行包本身不需要这些工具。Linux ARM64 没有实机，使用独立 Ubuntu 22.04 ARM64 用户空间、交叉编译器与 QEMU，未把它记录为实机通过。

| 项目 | Linux amd64 | macOS arm64 | Windows amd64 | Linux arm64 |
| --- | --- | --- | --- | --- |
| 两个 CLI / 原生后端构建 | 通过 | 实机通过 | 实机通过 | 交叉编译通过 |
| Go race / vet | 通过 | 实机通过 | 实机通过 | 未跑 race |
| 内嵌运行库加载 | glibc-only 根目录通过 | 重定位 dylib 通过 | 仅系统 PATH 通过 | QEMU 通过 |
| VNC 隔离桌面操作 | 通过 | 实机通过 | 实机通过 | QEMU 通过 |
| RDP 连接与截图 | Windows / shadow | Windows，经 SSH 转发 | Linux shadow，经 SSH 转发 | shadow，QEMU |
| RDP NLA 与输入 | shadow 认证/拒绝与输入通过 | 真实 Windows 认证/截图通过 | shadow NLA、点击/组合键通过 | shadow NLA、点击/组合键通过 |
| 安装、重装、缓存并发、配置保留 | 通过 | 通过 | 通过 | QEMU 通过 |

隔离 RFB 测试执行真实 CLI → HTTP → 原生 libgvnc 链路，检查 640×480 红色 PNG、中文及 emoji 的按下/释放、组合键、双击、拖拽、滚轮、终止请求后的按钮释放、单个/全部连接重连。运行目录刻意超过旧 socket 路径限制并包含中文和空格。测试不操作用户正在使用的真实 Linux 桌面。

Linux 原有 TigerVNC / FreeRDP shadow 回归通过，覆盖认证失败、TLS 默认拒绝/指纹匹配/不匹配/显式跳过、NLA、颜色、输入、尺寸变化、取消、部分批量失败及脱敏。Mac 原生 ARD UTF-16 代理对专项测试也通过。Windows 私有 ACL 测试确认 Everyone 可访问的文件被拒绝，缓存测试发现并修复了管理员默认所有者与目录创建/权限设置之间的并发窗口。

Mac 使用发行包连接真实屏幕共享，获取 1650×1050 PNG。Mac 到配置 Windows RDP 的直接 TCP 探测不可达；通过现有 SSH 会话临时转发协议流后，RDP 认证、状态和 1600×900 PNG 通过。测试结束关闭临时连接与转发，不在产品中新增隧道命令。

Windows 与 ARM64 连接独立 Linux shadow NLA 桌面，取得 640×480 PNG；点击和 CTRL+A 在服务端事件日志得到确认。`type Hello` 命令返回成功，但 shadow 事件日志没有提供逐字符确认，因此本轮不将其描述为 RDP 文本逐字符验收。VNC 隔离桌面的 Unicode 已逐事件验证。

Windows 在本机回连自身 RDP 时返回 `AUTH_FAILED`，与从 Linux/Mac 连接同一 Windows 的结果不同；此场景单独记录，不以 shadow 通过掩盖。企业 CA、域/SSO、VNC X509 和所有“本机 × 远端桌面”组合仍需扩展验收。

发行包分别包含 Linux x64 41 个、Linux arm64 41 个、Mac 25 个、Windows 42 个原生库。Mac 最低系统版本从 Mach-O 部署命令推导为 15.0；包使用 ad-hoc 签名，尚无 Developer ID/公证和下载隔离属性测试。GitHub 四平台工作流已配置但未触发，不能把本地执行结果写成 CI 已通过。

升级注意：本次 HTTP worker 与旧 Unix socket worker 不互通。先用旧 ctl 断开，再使用新 CLI 重连；令牌仅保存在私有 endpoint 文件，不进入 ctl 查询输出。

### 最终 Windows 重建复测的 ASR 拦截

Windows 早先的完整安装/VNC 操作测试与 RDP shadow NLA 测试确实通过。最终重建后再次执行安装测试，`ctl doctor` 通过，但启动 `remote-computer-use.exe --version` 被系统拒绝；此前通过的旧 EXE 随后也遭到相同拒绝。因此上表 Windows 的“通过”描述的是拦截发生前的结果，**最终包当前在该机器上不能完成安装复测**。

文件 ACL 检查确认当前用户具备 FullControl，未发现文件权限错误。Windows Defender Operational 日志记录事件 1121，规则 GUID 为 `C1DB55AB-C21A-4637-BB3F-A12568109D35`；对应微软的 [Use advanced protection against ransomware](https://learn.microsoft.com/en-us/defender-endpoint/attack-surface-reduction-rules-reference#use-advanced-protection-against-ransomware)。这是已观察到的 ASR 拦截，不据此声称文件被确认含有恶意代码，也不能在没有进一步证据时断言具体触发行为。

最终 Windows 包中 use.exe 的 SHA-256：`3d3f5071b6708833d55aa5f603d06edceb3cb50a9c2580476a586666c13f5de1`。本轮未关闭 Defender、未添加排除项、未修改服务端认证策略。Windows 开箱即用验收仍需解决正式签名/文件信誉或由机器管理员审核信任策略，再重跑测试；当前产物仅作为开发验证包交付。

### Windows VNC 专项复查与拦截范围修正

2026-09-30 后续复查中，同一已解压目录、同一 EXE 的完整 VNC 测试再次通过，涵盖 PNG/颜色、Unicode、键鼠、取消释放及重连；PowerShell 直接启动也恢复成功。但从发行包新解压到临时目录的安装测试仍在 `use.exe --version` 被拒绝，并新增同一 ASR 规则的事件 1121。因此应区分“Windows 原生 VNC 功能已通过”与“新安装流程仍存在 ASR 拦截”，不能笼统表述为该 EXE 始终无法启动。

版本查询不会释放或加载协议 DLL，实际加载库的 doctor 通过，目前没有证据把拒绝归因于内嵌 DLL。具体观察、推断边界和后续方向见 [Windows 启动拦截分析](windows-asr.md)。Mac 的公证也不作为当前终端安装与运行的前置要求，分发方式的验收边界见 [安装说明](installation.md#windows-与-macos)。

### 首版 0.1.0 产物验证

2026-09-30，为首版重新构建四个平台的 CLI，版本号均为 `0.1.0`，没有直接重命名 `dev` 包。Linux ARM64 复用此前验证过的 41 个原生库载荷，在隔离源码目录交叉编译 Go CLI，并在 Ubuntu 22.04 ARM64 + QEMU 中运行；其他三个平台使用各自原生环境生成包。

| 检查 | Linux x64 | Linux arm64 | macOS arm64 | Windows x64 |
| --- | --- | --- | --- | --- |
| 版本包原生库 doctor | 通过 | QEMU 通过 | 通过 | 通过 |
| 离线安装、重装、缓存损坏拒绝、配置保留、Skill 安装 | 通过 | QEMU 通过 | 通过 | 通过 |
| 完整 VNC 隔离测试：PNG/颜色、Unicode、键鼠、取消释放、重连 | 通过 | QEMU 通过 | 通过 | 通过 |
| 归档结构、平台清单与 SHA-256 | 通过 | 通过 | 通过 | 通过 |

本机 `make test`（race、vet）通过。归档不包含真实配置、端点令牌或桌面截图，包含 README、CHANGELOG、首版发布说明和第三方许可资料。最终文档同步后重新计算归档 SHA-256，并确认两个 CLI 的内容哈希不变。

Windows `0.1.0` 安装测试本轮未再触发此前的 ASR 拦截；未修改 Defender 策略或添加排除项。不能把此次通过解释为误拦截原因已经解决。macOS 未完成 Developer ID 签名、公证及浏览器下载隔离路径验收。本轮未重复真实桌面输入或 RDP 测试，沿用前述功能验证边界；GitHub 工作流尚未执行，当前仅完成本地发布准备。

首版许可核对发现 Windows 的系统 gdk-pixbuf 间接引入 TIFF/JBIG。改用固定版本 2.42.12、禁用全部图像加载器的最小 DLL 后，Windows 库数从 42 降至 32；最终包再次通过完整安装与 VNC 测试，清单确认不含 TIFF/JBIG。打包器新增拒绝 libjbig 的检查，项目采用 MIT，补齐 Go 依赖和标准 GNU 许可文本。

四个平台共归集 69 项去重后的精确原生源码记录，包含 Ubuntu `.dsc` 及其校验过的源码/补丁、MSYS2 完整源码包，以及 Homebrew 原始源码、已安装配方、资源和补丁。Glib 配方的瓶装哈希、livecheck 和顶层注释差异不影响构建；源码构建部分匹配后按固定仓库 revision 取得文件补丁。源码收集索引中无失败项。对应源码包还包含项目源码、构建脚本和 Go 模块源码，须与二进制同处发布。

### 跨命令保持输入（0.1.1-dev）

2026-09-30，针对无法在 `drag` 中插入 Esc 的使用反馈，新增 pointer-down/up、key-down/up 和 release-input，复用原有 VNC/RDP 输入 ABI。

- `make test`：race、vet 通过；覆盖跨命令状态、已有修饰键保留、多鼠标按钮、幂等释放、30 秒空闲期限、尺寸变化、取消、部分发送失败、释放重试及 watchdog 与操作串行化。
- `make integration-rdp-test`：隔离 TigerVNC 和 FreeRDP shadow 真协议测试通过。桌面事件日志确认移动和 Esc 按下时 Button1 仍保持，Shift+拖动同时带有修饰键和按钮状态；截图不松开鼠标，显式释放和正常断开均收到 up。已有认证、证书、截图、输入、取消和尺寸变化测试通过。
- `make release VERSION=0.1.1-dev` 与 `make package-test VERSION=0.1.1-dev`：Linux x64 内嵌依赖自检、离线安装、配置保留、Skill 安装与冲突检查通过。Skill frontmatter 校验通过。
- Windows x64、Linux arm64、macOS arm64 的 Go CLI 交叉编译通过；本次未重新做这些本机平台或 Mac/Windows 远端的新输入实机验收，不将 Linux 回环结果等同于实机验证。

本地开发包和 Codex/OpenCode Skill 已升级，原配置保留，原有 Linux/Mac 就绪连接重连以加载新 worker，Windows 保持未连接。尚未发布 GitHub Release；原有 0.1.0 发行归档不变。
