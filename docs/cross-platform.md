# 本机跨平台支持设计

状态：设计与可行性检查，2026-09-30。本文描述下一阶段目标，**不表示四个平台已经可以运行或已经发布**。

## 目标矩阵

用户指定的本机平台映射为四个 Go/发行目标。远端协议和系统矩阵独立于本机平台：每个本机发行包最终都应支持 VNC 和 RDP，能够连接远端 Linux、macOS、Windows。

| 本机目标 | Go target | 原生库 | 发行格式 | 初始验收基线 |
| --- | --- | --- | --- | --- |
| Windows x64 | windows/amd64 | DLL | zip + install.ps1 | Windows 10 22H2 / Windows 11 x64 |
| Linux x64 | linux/amd64 | ELF .so | tar.gz + install.sh | glibc 2.35+，沿用 Ubuntu 22.04 |
| Linux arm64 | linux/arm64 | ELF .so | tar.gz + install.sh | glibc 2.35+，Ubuntu 22.04 arm64 |
| macOS arm64 | darwin/arm64 | Mach-O .dylib | tar.gz + install.sh | 暂以 macOS 14+ 为发行目标，待依赖部署版本检查确认 |

macOS 的最低版本不能只由 Go 决定：Go 1.25 的最低要求是 macOS 12，但实际发行包还受所链接的原生库部署版本影响，必须检查每个 Mach-O 的最低系统版本。[Go 1.25 发布说明](https://go.dev/doc/go1.25)

继续保持两个 CLI、现有命令及配置字段。`go:embed` 保留，各目标仅携带本平台的依赖，不把四个平台的库塞进每个程序。发行文件统一为 `remote-computer-use-<version>-<goos>-<goarch>`。

## 当前代码检查结果

对当前提交进行不含内嵌库的 ctl 交叉编译检查：

| 目标 | 观察到的结果 | 含义 |
| --- | --- | --- |
| Linux amd64 | 既有构建、安装、真实协议测试已通过 | 当前唯一完整验证的本机平台 |
| Linux arm64 | `daemon.go` 使用的 `syscall.Dup2` 不存在，编译失败 | 即使同属 Linux，也需要处理架构差异 |
| macOS arm64 | 生成 arm64 Mach-O 成功 | 后端仍显式拒绝非 Linux，不能据此称为可运行 |
| Windows amd64 | `runtimebundle` 使用的 `syscall.Stat_t` 不存在，编译失败 | 还存在 flock、Setsid、Dup/Dup2、Unix 权限等后续问题 |

原生层还使用 pthread、unistd/usleep、arpa/inet.h，以及仅面向 ELF 的导出声明。发行脚本固定 Linux amd64，并依赖 ldd、patchelf、dpkg；安装脚本使用 GNU `mv -T`，不能直接在 macOS 执行。

已只读确认现有 SSH 连接中有 macOS arm64 和 Windows x64 可用于后续验收；未安装依赖、上传程序或更改这些主机。Mac 现有 Go 为 1.24.9，低于项目要求；原生构建工具与依赖不齐全。尚未确认原生 Linux arm64 测试机。

## 主要技术决策

### 1. 复用协议实现，拆分本机平台层

继续使用 libgvnc + FreeRDP，不为了跨平台重写 VNC/RDP。四个目标共用输入、截图、状态机与 C ABI。Homebrew 提供 gtk-vnc/FreeRDP，MSYS2 提供 UCRT64 对应包，说明现有库有可利用的构建路径；正式产物仍需自行裁剪和固定版本，不能直接打包完整 GUI 客户端及依赖。[Homebrew gtk-vnc](https://formulae.brew.sh/formula/gtk-vnc)、[Homebrew FreeRDP](https://formulae.brew.sh/formula/freerdp)、[MSYS2 包目录](https://packages.msys2.org/packages/)

新增内部 `host` 平台层，用 build tags 隔离系统调用，负责：私有目录与文件检查、互斥锁、IPC 监听与拨号、后台进程启动、启动应答管道和原生日志隔离。应用层不再直接操作 syscall 或假设 `.sock` 文件存在。动态库名称和加载策略由 backend 的平台文件负责。

Windows 原生构建首期采用 MinGW-w64 UCRT64，可复用 winpthreads，不同时维护 MSVC 构建路径。补齐 `__declspec(dllexport)`、Winsock 地址接口、睡眠和标准流处理。Linux/macOS 保留 pthread。C ABI 继续使用定宽整数和显式缓冲区长度，检查 Windows LLP64 与 Unix LP64 的区别；仅在确实改变 ABI 时升级版本。

### 2. IPC 与后台生命周期按系统实现

| 项目 | Linux / macOS | Windows |
| --- | --- | --- |
| IPC | Unix domain socket | 本地 named pipe |
| 连接锁 | flock，持有到原生连接关闭 | 文件句柄独占或 LockFileEx，持有到关闭 |
| 目录与配置保护 | UID、0700 / 0600 | 当前用户 SID 与 DACL |
| worker 启动 | 脱离启动终端的进程会话 | 无新控制台窗口，显式限制句柄继承 |
| 原生日志隔离 | 保存启动管道后重定向 fd | 分别处理 Win32 标准句柄与 C 运行时标准流 |

Windows named pipe 优先使用 `go-winio`，显式配置仅允许当前用户及必要系统主体的 DACL，并拒绝远程管道客户端。命名包含用户 SID 和规范化运行目录的哈希，支持并行测试和多用户隔离。默认权限不能视为足够私有。[go-winio](https://github.com/microsoft/go-winio)、[Microsoft 命名管道权限](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)

查询与 `disconnect --all` 改为发现安全的连接状态记录，再实际探测对应 IPC；不能只依赖扫描 `.sock`，也不能把旧状态文件当作活连接。保留“断开等待后端清理完成”的语义。

凭据仍仅经私有启动管道传递，不加入命令行或普通状态文件。Windows 的静默要求是 worker 不弹出控制台，并不意味着用户在终端调用的 CLI 没有 stdout。

macOS 默认 TMPDIR 往往较长，需要选用短且按 UID 隔离的 socket 目录；现有 58 字节目录限制要按实际完整 socket 路径处理。不通过关闭路径长度检查解决。

### 3. 配置路径保持熟悉，运行数据遵循平台

所有平台继续使用 `~/.remote-computer-use/config.toml`；Windows 的 `~` 是用户配置目录。既有 `REMOTE_COMPUTER_CONFIG`、`REMOTE_COMPUTER_LIB_DIR`、`REMOTE_COMPUTER_RUN_DIR`、`REMOTE_COMPUTER_CACHE_DIR` 保持可用。

缓存默认采用 Go 的平台缓存目录：Linux 的 XDG cache、macOS 的用户 Library/Caches、Windows 的 LocalAppData。Windows 配置和缓存不能通过 `FileMode & 0077` 判断私密性，也不能认为 `chmod 600` 已保护文件；必须真正设置和检查 ACL、所有者及重解析点。安装器只创建项目自己的私有目录，不递归修改整个用户目录权限。

Windows 的同名文件大小写、保留文件名、冒号/备用数据流、反斜杠、junction 等需要进入归档与缓存校验规则。缓存继续逐文件校验、按内容哈希隔离、完整目录原子发布；升级不覆盖已加载 DLL，不自动删除旧缓存。

### 4. 运行库分平台收集和加载

| 平台 | 依赖收集与重定位 | 系统提供的部分 |
| --- | --- | --- |
| Linux | ELF DT_NEEDED 闭包，逐库 `$ORIGIN` RUNPATH | glibc 与加载器 |
| macOS | Mach-O 依赖闭包，改为 `@loader_path` / 合适的 install name | libSystem、系统 Frameworks |
| Windows | PE imports 依赖闭包，随包携带第三方 DLL | Windows 系统 DLL、API sets 与目标系统已有的 UCRT |

Windows 当前 FFI 封装调用普通 `LoadLibrary`，不能保证从释放目录解析传递依赖。平台加载器应使用绝对路径和受限的 `LoadLibraryExW` 搜索范围（库目录及 System32），不能靠修改工作目录或把缓存目录加入全局 PATH。[Microsoft DLL 搜索顺序](https://learn.microsoft.com/zh-cn/windows/win32/dlls/dynamic-link-library-search-order)

macOS 修改 Mach-O 后重新签名，**签名完成后**才计算哈希并嵌入。开发版可以使用 ad-hoc 签名；正式开箱即用发行需另行完成 Developer ID 签名、公证及真实下载场景测试，不能把 ad-hoc 视为已满足分发信任要求。[Apple 代码签名格式](https://developer.apple.com/documentation/xcode/using-the-latest-code-signature-format)、[Apple 公证要求](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)

不把系统证书、用户凭据和构建机的 Homebrew/MSYS2 路径固化进包。各发行包继续附第三方许可和版本来源清单，检查不存在指向开发机器的依赖。

### 5. 系统证书信任必须单独处理

当前 RDP 验证通过 OpenSSL 默认路径加载 CA。在 Windows/macOS 上，这不等于自动使用系统信任库，也可能指向构建机的安装前缀。

目标是保持一致的证书策略：Windows 使用系统证书链验证能力，macOS 使用 Security.framework 的 TLS 信任验证，Linux 保持现有系统 CA 验证；可在 C 适配层封装平台差异，保留“不从 C 回调 Go”的现有边界。VNC 的 GnuTLS 系统信任行为也需按实际构建验证，不能由函数名推断平台支持范围。

已有固定指纹和显式 `tls_verify = false` 语义保持不变。默认验证不可因平台移植自动变为跳过；系统 CA 尚未实现或测试通过的平台不能标为完整兼容。

### 6. 安装入口保持简单

Linux/macOS 提供 POSIX 兼容的 `install.sh`；Windows 提供 PowerShell `install.ps1`，正常安装不需要管理员权限、MSYS2、Go、Homebrew 或编译器。

Windows 安装使用版本目录和普通入口程序路径，避免要求开发者模式才能创建符号链接。已加载 EXE/DLL 的升级和旧版本保留采用 Windows 可用的文件操作。配置永不覆盖；Agent Skill 可选安装；路径包含中文、空格时正常工作。PATH 修改为显式选项或给出当前会话命令，不隐式改系统 PATH。

## 实施顺序与阶段提交

1. **平台层**：分离文件保护、IPC、锁、worker 启动和日志隔离；四目标 Go 编译进入 CI；Linux amd64 既有功能持续回归。
2. **Linux arm64**：处理 Dup2/Dup3 差异，打包器识别原生架构；使用 Ubuntu 22.04 arm64 构建原生依赖，验收最小运行环境和真实协议。QEMU 可用于早期检查，不代替原生稳定性验证。
3. **macOS arm64**：构建无显示依赖的 libgvnc/FreeRDP，dylib 重定位与签名、系统信任、POSIX 安装脚本，使用现有 Mac 验收。
4. **Windows x64**：named pipe、ACL、进程/标准流、受限 DLL 加载、UCRT64 原生构建、系统信任、PowerShell 安装器，使用现有 Windows 验收。
5. **发行矩阵**：各平台原生 runner 分别构建和测试，再汇总四种归档与校验和；发布级签名、公证独立记录。实际完成的平台才进入正式发行列表。

Linux arm64 与 macOS 的优先级高于 Windows，是因为可以复用更多已有进程和文件语义；Windows 工作仍从平台层阶段开始，不等其他平台全部结束后才设计。

## 验收条件

每个目标逐级标记“编译通过 / 后端加载通过 / 协议功能通过 / 自包含发行通过”，不可混为一个“支持”。

- 原生系统运行 `doctor`；没有 FreeRDP、libgvnc、Homebrew、MSYS2 和构建目录仍能加载库。
- 配置保留、首次安装、重复安装、升级、中文/空格路径、缓存首次并发释放、错误权限与损坏拒绝。
- worker 在发起命令退出后继续存在，不显示本机 GUI/控制台；退出和立即重连正常，取消释放键鼠。
- 跨本机用户无法读取连接秘密或控制 IPC；状态与日志继续脱敏。
- 每个本机平台都跑 VNC 与 RDP 协议夹具：认证/TLS/错误证书、PNG、Unicode、键鼠、尺寸变化、断线；新增 Windows/macOS IPC 测试不依赖 Linux 的 socket 文件行为。
- 最终覆盖“4 个本机目标 × 3 类远端桌面”，至少逐组合验证连接、截图和输入；原有 Mac emoji 与 ARD 水平滚轮回归保留。
- 正式发行检查实际下载后的系统执行策略与签名，不以开发机运行成功代替安装验收。

本轮只完成调查、设计和编译探测，没有改变已发布接口或将未验证的平台宣布为可用。
