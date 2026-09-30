# 本机跨平台支持

状态：已实现四个构建目标，2026-09-30。Linux x64、macOS arm64、Windows x64 使用现有机器验证；Linux arm64 使用 Ubuntu 22.04 ARM64 用户空间和 QEMU 验证，尚无 ARM Linux 实机验收。开发包未在线发布。Windows 在先前测试通过后，最终重建复测被本机 Defender ASR 拦截；当前不能宣称 Windows 包开箱即用，详见验证记录。

## 平台矩阵

| 本机目标 | 原生库 | 发行格式 | 当前验证基线 |
| --- | --- | --- | --- |
| Windows amd64 | DLL | zip + install.ps1 | Windows 10 22H2 x64 |
| Linux amd64 | ELF .so | tar.gz + install.sh | Ubuntu 22.04，glibc 2.35+ |
| Linux arm64 | ELF .so | tar.gz + install.sh | Ubuntu 22.04 ARM64，QEMU |
| macOS arm64 | Mach-O .dylib | tar.gz + install.sh | macOS 15.7.7；当前依赖要求 macOS 15.0+ |

macOS 最低版本由每个 dylib 的 `LC_BUILD_VERSION` / `LC_VERSION_MIN_MACOSX` 推导，记录在包内 `runtime-manifest.json`。当前 Homebrew 依赖需要 15.0，不能把最初规划的 14.0 当作已支持。macOS 库采用 ad-hoc 签名，尚未完成 Developer ID 签名、公证及真实下载隔离属性验收。

本机平台与被操作桌面的系统相互独立。每个目标都包含 VNC 与 RDP 后端；“后端可加载”“协议测试通过”“所有目标桌面组合实测”是不同层级，详细边界见 [验证记录](validation.md)。

## 架构与系统差异

继续复用 libgvnc 与固定 FreeRDP 3.25.0。Go CLI 使用 `CGO_ENABLED=0`，通过动态 FFI 加载原生适配库；C ABI 不变，不从 C 回调 Go。Windows 原生构建采用 MinGW-w64 UCRT64，不引入 MSYS 运行时依赖。

| 项目 | Linux / macOS | Windows |
| --- | --- | --- |
| IPC | 认证的 loopback HTTP | 相同接口 |
| 生命周期锁 | flock + close-on-exec | 独占文件句柄，拒绝重解析点 |
| 目录与文件保护 | 当前 UID，0700 / 0600 | 当前 SID、受保护 DACL，保留 SYSTEM/Administrators |
| 后台进程 | Setsid，重定向标准流 | 无新控制台窗口，分别隔离 Win32 与 C 标准流 |
| 动态库加载 | 平台 dlopen | 绝对路径 LoadLibraryEx，限定库目录与 System32 |
| RDP 系统信任 | Linux OpenSSL CA；macOS Security.framework | Windows 证书链与 SSL 策略 API |

HTTP 取代最初设计的 Unix socket / Windows named pipe，解决跨平台 IPC 差异，并为操作节点转发留下接口边界。每连接随机端口、独立 256 位令牌、禁止代理及重定向；详细约束见 [HTTP 接口设计](http-ipc.md)。当前不开放远端 API。

Windows 私有目录通过创建时的 security descriptor 原子设置权限，避免并发首次释放看见尚未保护的目录；文件显式设置所有者，兼容管理员会话默认以 Administrators 为所有者的行为。已有不安全目录不被自动修正。缓存文件名拒绝大小写冲突、设备名、冒号、路径分隔符、尾随点/空格和重解析点。

VNC 的认证范围沿用现有实现；VeNCrypt X509 与企业证书组合仍需专项实测。RDP 的指纹固定及 `tls_verify = false` 语义保持不变，默认不跳过证书校验。

## 运行库与安装

`scripts/bundle-runtime.py` 在目标平台递归收集依赖闭包，生成带 SHA-256 的 manifest 和压缩载荷。`go:embed` 只嵌入本目标的库，由 ctl 在首次自检/连接时释放。use 不重复携带同一份载荷。

- Linux：保留宿主 glibc/加载器，其余库设为 `$ORIGIN` RUNPATH；拒绝高于 glibc 2.35 的符号需求。
- macOS：保留系统 Frameworks/libSystem，第三方依赖改为 `@loader_path`，移除构建路径，重新签名后计算哈希。
- Windows：读取 PE imports，保留系统 DLL/API sets，其余 DLL 放入同一私有目录；运行时不依赖 MSYS2/PATH。

包内保留库来源、版本与许可文本。Homebrew 缺失的许可文件从安装时 formula 记录的源码 URL 获取，并校验其 SHA-256；MSYS2 保留包信息和包内许可，标准 GNU 许可文本作为补充。所有下载只发生在构建阶段，安装与运行不下载依赖。

Linux/macOS 安装器使用版本目录和原子链接切换；Windows 使用版本目录及 `.cmd` 入口，不要求符号链接权限。三个安装入口都保留已有配置，可选安装 Agent Skill，不隐式修改 PATH。升级到 HTTP 版前须用旧 ctl 断开旧 Unix socket worker。

## 构建与验证

Linux 的构建步骤见 README。macOS 示例：

```sh
brew install cmake ninja gtk-vnc openssl@3 pkgconf
scripts/build-freerdp.sh
PKG_CONFIG_PATH="$PWD/.cache/freerdp/lib/pkgconfig" make native
make build test native-input-test
go run ./tests/portable
make release VERSION=dev
```

Windows 在 MSYS2 UCRT64 shell 中安装 gcc、cmake、ninja、pkgconf、python、gtk-vnc、openssl 对应的 `mingw-w64-ucrt-x86_64-*` 包及 make/tar/curl，然后执行相同的 FreeRDP/native/build/release 步骤。构建脚本处理 MinGW 缺少 C11 `threads.h`、FreeRDP unwind 分支和 GCC 14+ 对旧源码指针类型诊断的变化；协议代码未因此改变。

`tests/portable` 提供隔离的 RFB 桌面，使用真实 CLI 和原生 VNC 库检查 PNG 颜色、Unicode、键鼠、请求取消、重连、长路径及中文路径，不向用户真实桌面发送测试文字。Linux 的 TigerVNC / FreeRDP shadow 测试继续覆盖认证、证书、增量画面和尺寸变化。`tests/package.py` 与 `tests/package-windows.py` 分别验证安装、重装、缓存和自包含运行。

手动 GitHub Actions 工作流提供四目标构建矩阵；尚未在 GitHub 上触发。runner 标签按 [GitHub 官方列表](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) 配置，实际通过情况以运行结果为准。Linux arm64 的 QEMU 验证不能替代原生稳定性、性能和完整桌面矩阵验收。
