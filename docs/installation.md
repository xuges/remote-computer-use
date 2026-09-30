# 安装与发行包

## 使用自包含发行包

开发包覆盖 Linux amd64/arm64（glibc 2.35+）、Windows amd64、macOS arm64（当前依赖要求 15.0+）。运行不需要 Go、编译器、FreeRDP、libgvnc、Python 或管理员权限。Linux ARM64 仅完成 QEMU 验证，Mac 包尚未完成 Developer ID 签名与公证；Alpine/musl 不支持。详细层级见 [跨平台验证](cross-platform.md)。

取得 `.tar.gz` 及同名 `.sha256` 文件后，在同一目录执行（将 `VERSION` 替换为实际版本）：

```sh
sha256sum -c remote-computer-use-VERSION-linux-amd64.tar.gz.sha256
tar -xzf remote-computer-use-VERSION-linux-amd64.tar.gz
cd remote-computer-use-VERSION-linux-amd64
./install.sh
# 也可同时安装一个 Agent 的 Skill：
# ./install.sh --agent codex
# ./install.sh --agent opencode
# ./install.sh --agent claude-code
```

安装脚本完全离线：先检查内嵌运行库，再安装两个 CLI，首次创建权限为 0600 的示例配置；已有配置一律保留。仍需自行填写真实连接信息并启用远端 VNC/RDP 服务。默认命令链接位于 `~/.local/bin`，若 PATH 未包含它，安装器会打印应添加的命令。Agent 的运行环境也须包含该 PATH。

```sh
export PATH="$HOME/.local/bin:$PATH"
remote-computer-ctl doctor
# 编辑 ~/.remote-computer-use/config.toml
remote-computer-ctl connect mac
remote-computer-use --conn mac screenshot --output screen.png
```

`--prefix /absolute/path` 更改程序安装位置，`--bin-dir /absolute/path` 更改命令链接位置；配置仍默认使用 `~/.remote-computer-use/config.toml`。同一安装命令可以重复执行。遇到同名无关命令或内容不同的 Skill 会停止并保留原文件，不修改 shell 启动文件或 Agent 的其他设置。

也可以直接运行解压后的 `bin/`，或只把两个 CLI 拷到自己的工具目录。`ctl` 自带协议运行库，`use` 通过带令牌的本机 HTTP 调用它启动的 worker，不需要重复携带同一份库。

## Windows 与 macOS

macOS 使用对应的 `darwin-arm64.tar.gz`，校验命令为 `shasum -a 256 -c 包名.sha256`，安装参数与 Linux 相同。当前开发包使用 ad-hoc 签名，不能视为已完成公开分发的系统信任验收。

本轮 Windows 包在早先测试通过后，最终复测被 Defender ASR 高级勒索软件防护规则拦截，当前不能视为开箱即用发行版。安装器不更改安全策略；应由机器管理员审核签名/信誉与信任策略，详见 [验证记录](validation.md)。

Windows 开发包使用 `windows-amd64.zip`，通过 `Get-FileHash -Algorithm SHA256 包名.zip` 对照 `.sha256`，解压后执行：

```powershell
powershell -File .\install.ps1
# 可选：-Prefix C:\Tools\remote-computer-use -Agent codex
```

默认入口为 `~/.remote-computer-use/bin/*.cmd`，实际 EXE 位于按内容哈希命名的版本目录。无需符号链接权限；把安装器打印的 bin 目录加入终端和 Agent 的 PATH。已有配置保留；新配置显式设置当前用户 SID、SYSTEM 和 Administrators 的 DACL。PowerShell 执行策略由本机环境控制，安装器不改变它。

## 自检与运行库缓存

```sh
remote-computer-ctl doctor --json
```

`doctor` 不读取连接配置，不访问远端，也不验证远端凭据。它检查本机 VNC/RDP 适配库及依赖能否加载、ABI 是否兼容。JSON 只输出版本、平台、运行库来源和公开错误码。首次自检或连接会离线解压内嵌库。

加载优先级：

1. 显式的绝对路径 `REMOTE_COMPUTER_LIB_DIR`，用于开发或替换兼容的动态库。
2. 发行版中的内嵌运行库，释放到系统用户缓存目录的 `remote-computer-use/native/<内容哈希>`：Linux 为 `$XDG_CACHE_HOME` 或 `~/.cache`；macOS 为 `~/Library/Caches`；Windows 为 `%LOCALAPPDATA%`。
3. 不含内嵌资源的开发版，从可执行文件所在目录加载适配库。

`REMOTE_COMPUTER_CACHE_DIR=/absolute/path` 可覆盖缓存根目录。Unix 缓存目录权限为 0700、库文件为 0600；Windows 使用私有 DACL。文件系统必须允许动态库映射可执行页，`noexec` 挂载不可用。缓存按完整清单内容哈希隔离，不以可重复的 `dev` 版本号作为目录名。并发首次启动只发布完整且校验通过的目录，每次 worker 启动都会校验文件 SHA-256。损坏、权限过宽或符号链接缓存会被拒绝，不加载不完整文件。

`BACKEND_UNAVAILABLE` 常见排查顺序：确认发行包平台和 glibc 版本；取消不需要的 `REMOTE_COMPUTER_LIB_DIR`；检查缓存目录是否可写、挂载是否允许执行；若缓存损坏，先断开所有连接，再删除该项目的缓存目录后重试 `doctor`。不要在有活动 worker 时清理缓存，因为原生库可能延迟加载其他模块。

glibc 和动态加载器由操作系统提供，发行包不替换它们。证书校验使用本机系统 CA；启用 CA 验证时须有可用的系统 CA 信任库。DNS、系统证书、企业 Kerberos/SASL 扩展配置也不属于内嵌资源。域环境、额外认证插件及其他发行版需分别验收。

## 升级与卸载

重新运行新包的 `install.sh` 即可升级。程序按内容哈希存放在 `~/.remote-computer-use/releases/`，`current` 链接切换到新版本；旧版本与缓存保留，避免升级时删除活动 worker 的资源。同一 IPC 版本下已有连接继续使用原 worker，断开并重新连接后才使用新版本。**从旧 Unix socket 版升级本次 HTTP 版前，请先使用旧 ctl 断开连接，再替换两个 CLI。** Windows 通过更新 `.cmd` 入口选择新版本，旧 EXE/DLL 同样保留。

卸载时先执行 `remote-computer-ctl disconnect --all`，再删除安装器创建的两个命令链接、程序的 `bin/`、`current`、`releases/` 和项目缓存目录。保留 `config.toml`，除非确实要删除连接配置；按需删除选定 Agent 的 `remote-computer-use` Skill 目录。

## 构建发行包

开发依赖仍需在构建机器上安装；运行用户不需要安装它们。按照 README 构建无 X11 的两个适配库后：

```sh
make release VERSION=dev
make package-test VERSION=dev
# Windows UCRT64 shell:
# go build -o build/portable.exe ./tests/portable
# python3 tests/package-windows.py dist/remote-computer-use-dev-windows-amd64.zip build/portable.exe
# 可选：仅在能使用 sudo 的测试机运行最小根目录验证。
python3 tests/package.py dist/remote-computer-use-dev-linux-amd64.tar.gz --chroot
```

`make release` 在当前平台收集适配库的依赖闭包：Linux ELF 设置 `$ORIGIN`，Mac dylib 设置 `@loader_path` 并重新签名，Windows PE imports 的第三方 DLL 放在同一目录。排除系统库，验证闭包后生成归档与哈希清单。Go 以 `embedded` build tag 将资源编入 ctl。打包器拒绝高于 glibc 2.35 的符号需求和意外引入的 X11/音频库。生成的原生资源、发行包不提交 Git。

发行包包含第三方版权与许可文本、库清单、Ubuntu、Homebrew 或 MSYS2 包版本和源码获取位置。构建器会对不明来源库停止打包。Mac 缺失许可文本时只在构建阶段下载并校验对应源码；最终安装包仍完全离线。固定 FreeRDP 版本由 `scripts/build-freerdp.sh` 校验来源归档；本机系统依赖的补丁版本记入发行包清单，发行包会随重建更新。

当前产物在本机 `dist/` 生成，尚未发布下载地址。GitHub 手动构建工作流会上传可下载的构建产物，不自动创建或发布 Release。
