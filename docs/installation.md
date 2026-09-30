# 安装与发行包

## 使用自包含发行包

首批包支持 **Linux x86_64、glibc 2.35 或更新版本**，基于 Ubuntu 22.04 构建。本机不需要 Go、编译器、FreeRDP、libgvnc、X11 或 Python，也不需要 root。Alpine/musl、本机 macOS/Windows、ARM64 暂未提供发行包；这不影响操作远端 Mac、Windows 和 Linux。

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

也可以直接运行解压后的 `bin/`，或只把两个 CLI 拷到自己的工具目录。`ctl` 自带协议运行库，`use` 通过本机 socket 调用它启动的 worker，不需要重复携带同一份库。

## 自检与运行库缓存

```sh
remote-computer-ctl doctor --json
```

`doctor` 不读取连接配置，不访问远端，也不验证远端凭据。它检查本机 VNC/RDP 适配库及依赖能否加载、ABI 是否兼容。JSON 只输出版本、平台、运行库来源和公开错误码。首次自检或连接会离线解压内嵌库。

加载优先级：

1. 显式的绝对路径 `REMOTE_COMPUTER_LIB_DIR`，用于开发或替换兼容的动态库。
2. 发行版中的内嵌运行库，释放到 `$XDG_CACHE_HOME/remote-computer-use/native/<内容哈希>`；未设置 XDG 时使用 `~/.cache`。
3. 不含内嵌资源的开发版，从可执行文件所在目录加载适配库。

`REMOTE_COMPUTER_CACHE_DIR=/absolute/path` 可覆盖缓存根目录。缓存目录权限为 0700、库文件为 0600；文件系统必须允许动态库映射可执行页，`noexec` 挂载不可用。缓存按完整清单内容哈希隔离，不以可重复的 `dev` 版本号作为目录名。并发首次启动只发布完整且校验通过的目录，每次 worker 启动都会校验文件 SHA-256。损坏、权限过宽或符号链接缓存会被拒绝，不加载不完整文件。

`BACKEND_UNAVAILABLE` 常见排查顺序：确认发行包平台和 glibc 版本；取消不需要的 `REMOTE_COMPUTER_LIB_DIR`；检查缓存目录是否可写、挂载是否允许执行；若缓存损坏，先断开所有连接，再删除该项目的缓存目录后重试 `doctor`。不要在有活动 worker 时清理缓存，因为原生库可能延迟加载其他模块。

glibc 和动态加载器由操作系统提供，发行包不替换它们。证书校验使用本机系统 CA；启用 CA 验证时须有可用的系统 CA 信任库。DNS、系统证书、企业 Kerberos/SASL 扩展配置也不属于内嵌资源。域环境、额外认证插件及其他发行版需分别验收。

## 升级与卸载

重新运行新包的 `install.sh` 即可升级。程序按内容哈希存放在 `~/.remote-computer-use/releases/`，`current` 链接切换到新版本；旧版本与缓存保留，避免升级时删除活动 worker 的资源。已有连接继续使用原 worker，断开并重新连接后才使用新版本。

卸载时先执行 `remote-computer-ctl disconnect --all`，再删除安装器创建的两个命令链接、程序的 `bin/`、`current`、`releases/` 和项目缓存目录。保留 `config.toml`，除非确实要删除连接配置；按需删除选定 Agent 的 `remote-computer-use` Skill 目录。

## 构建发行包

开发依赖仍需在构建机器上安装；运行用户不需要安装它们。按照 README 构建无 X11 的两个适配库后：

```sh
make release VERSION=dev
make package-test VERSION=dev
# 可选：仅在能使用 sudo 的测试机运行最小根目录验证。
python3 tests/package.py dist/remote-computer-use-dev-linux-amd64.tar.gz --chroot
```

`make release` 收集适配库的 ELF 依赖闭包，排除 glibc/加载器，给每个库设置 `$ORIGIN` RUNPATH，验证没有依赖残留在构建目录，再生成压缩归档与哈希清单。Go 以 `embedded` build tag 将资源编入 ctl。打包器拒绝高于 glibc 2.35 的符号需求和意外引入的 X11/音频库。生成的原生资源、发行包不提交 Git。

发行包包含第三方版权与许可文本、库清单、Ubuntu 源码包版本和获取位置。构建器会对不明来源库停止打包。固定 FreeRDP 版本由 `scripts/build-freerdp.sh` 校验来源归档；本机系统依赖的补丁版本记入发行包清单，发行包会随重建更新。

当前产物在本机 `dist/` 生成，尚未发布下载地址。GitHub 手动构建工作流会上传可下载的构建产物，不自动创建或发布 Release。
