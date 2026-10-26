# remote-computer-use

让 AI 在本机通过 VNC / RDP 观察和操作远端桌面。两个 Go 命令行工具共用后台会话：

- `remote-computer-ctl`：查询、连接、断开，以及连接全部、断开全部。
- `remote-computer-use`：截图、移动、点击、拖拽、滚轮、组合键和文本输入。

与 [remote-shell](https://github.com/xuges/remote-shell) 独立互补：remote-shell 处理远程命令和文件，本项目处理 GUI。远端只需提供 VNC / RDP 服务，无需安装本项目的执行助手。本机不打开窗口，不依赖 DISPLAY、Wayland、Xvfb 或外部键鼠工具；截图来自协议解码后的内存画面。

当前版本 **v0.1.1** 提供统一命令、后台连接、内嵌协议运行库、跨命令保持输入及 Agent Skill。版本处于早期阶段，详细功能与限制见 [v0.1.1 发布说明](docs/releases/v0.1.1.md) 和 [首版发布说明](docs/releases/v0.1.0.md)。

项目自身代码采用 [MIT License](LICENSE)。自包含包中的第三方库保留各自许可；发布时同时提供对应源码包，详见 [第三方许可与源码说明](THIRD_PARTY.md)。

## AI 安装

让 AI 自己安装时，直接发送这句话：

> 请按照 https://github.com/xuges/remote-computer-use/raw/refs/heads/main/INSTALL.md 安装 remote-computer-use 技能。

## 支持平台与已知风险

以下是运行命令行工具的**本机平台**；被操作的远端需启用兼容的 VNC / RDP 服务。

| 本机平台 | 最低要求 / 验证范围 | 发行包后缀 |
| --- | --- | --- |
| Linux x64 | glibc 2.35+，已实机验证 | `linux-amd64.tar.gz` |
| Linux arm64 | glibc 2.35+，仅完成 QEMU 验证，尚待实机验收 | `linux-arm64.tar.gz` |
| macOS arm64 | macOS 15.0+，已在 15.7.7 验证 | `darwin-arm64.tar.gz` |
| Windows x64 | 已在 Windows 10 22H2 验证 | `windows-amd64.zip` |

> **macOS 未公证：** 当前包仅采用 ad-hoc 签名，未完成 Developer ID 签名与 Apple 公证。本机构建、SSH 传输后在终端安装运行已通过，但浏览器下载附带隔离属性的文件可能被 Gatekeeper 阻止，相关下载安装路径尚未验收。公证不作为当前 CLI 功能运行的前置条件；安装器不会自动移除隔离标记或关闭系统防护。
>
> **Windows 可能被 Defender 拦截：** 当前 EXE 未签名，此前开发包新解压后曾触发 Defender ASR，表现为启动时“拒绝访问”，甚至 `--version` 也无法执行。`0.1.0` 本轮安装和 VNC 截图、键鼠测试已通过，但不能保证其他机器或后续下载不再被拦截。请根据防护历史记录核对文件及拦截规则；安装器不会修改 Defender 策略。现有证据不支持将拦截归因于内嵌 DLL，详见 [Windows 拦截分析](docs/windows-asr.md)。

远端 Linux VNC、Mac VNC/ARD、Windows RDP 已完成连接、截图和键鼠实测。Apple ARD 水平滚动暂不支持；Windows 双轴滚动请分成两条命令。Windows 本机回连自身 RDP 的认证问题仍待解决，不能由其他 RDP 场景通过推断该场景可用。更多测试范围见 [验证记录](docs/validation.md)，总体架构见 [设计文档](docs/design.md)。

## 快速安装

从 [GitHub Releases](https://github.com/xuges/remote-computer-use/releases) 选择对应平台的 `remote-computer-use-0.1.1-<平台>` 包及同名 `.sha256` 文件，以 Releases 页面实际列出的资产为准。自包含包运行时无需安装 Go、FreeRDP、libgvnc 或显示组件，也不需要管理员权限。

Linux x64 示例（macOS 使用 `shasum -a 256 -c` 校验并选择 `darwin-arm64` 包）：

```sh
sha256sum -c remote-computer-use-0.1.1-linux-amd64.tar.gz.sha256
tar -xzf remote-computer-use-0.1.1-linux-amd64.tar.gz
cd remote-computer-use-0.1.1-linux-amd64
./install.sh
# 或同时安装 Agent Skill：./install.sh --agent codex
export PATH="$HOME/.local/bin:$PATH"
remote-computer-ctl doctor
# 编辑 ~/.remote-computer-use/config.toml 中的真实连接信息
```

首次安装自动创建私有示例配置（Unix 0600，Windows DACL），已有配置始终保留。运行库通过 `go:embed` 内嵌在 ctl 中，首次自检或连接时自动释放到私有缓存。安装过程完全离线，不修改 shell 启动文件。也可直接运行包内 `bin/` 的两个 CLI。

Windows 用 `Get-FileHash -Algorithm SHA256` 对照 `.sha256` 文件，解压 `.zip` 后执行：

```powershell
powershell -File .\install.ps1
# 或同时安装 Skill：powershell -File .\install.ps1 -Agent codex
```

将安装器打印的 `bin` 目录加入 PATH，再执行 `remote-computer-ctl doctor`。完整校验、安装参数、缓存和升级步骤见 [安装文档](docs/installation.md)，构建与发布步骤见 [发布指南](docs/releasing.md)。

## 从源码构建

要求 Go 1.25.8 或更新版本、C 编译器、CMake、pkg-config、libgvnc ≥ 1.3、FreeRDP ≥ 3.25。Go 使用 `github.com/goexlib/cgo` 动态加载 C 适配库，两个 CLI 使用 `CGO_ENABLED=0` 构建。**Go 二进制仍需要相应协议的原生共享库。**

Ubuntu 22.04 示例：

```sh
sudo apt-get update
sudo apt-get install -y build-essential cmake ninja-build pkg-config curl patchelf python3 \
  libgvnc-1.0-dev libglib2.0-dev libssl-dev libkrb5-dev zlib1g-dev \
  libjpeg-dev libpng-dev libicu-dev uuid-dev

# 发行版没有足够新的 FreeRDP 时，构建固定版本 3.25.0。
# 安装至固定私有目录，避免运行时依赖仓库里的缓存。
FREERDP_PREFIX="$HOME/.local/lib/remote-computer-use/freerdp" scripts/build-freerdp.sh
PKG_CONFIG_PATH="$HOME/.local/lib/remote-computer-use/freerdp/lib/pkgconfig" make native
make build
make install
export PATH="$HOME/.remote-computer-use/bin:$PATH"
```

构建脚本校验下载归档的 SHA-256，不执行系统安装。上述 `make install` 是开发版安装：将两个 CLI 和适配库复制到 `~/.remote-computer-use/bin`，可通过 `PREFIX=/absolute/path` 更改；不会创建或覆盖配置。开发版依赖原安装位置的 FreeRDP、libgvnc。要生成可搬运的内嵌运行库发行包，在两个无 X11 的适配库构建完成后执行：

```sh
make release VERSION=dev
make package-test VERSION=dev
```

开发时可直接使用 `bin/`。仅构建一种协议：

```sh
make native NATIVE_FLAGS='-DRCU_BUILD_RDP=OFF -DRCU_BUILD_VNC=ON'
# 或 -DRCU_BUILD_RDP=ON -DRCU_BUILD_VNC=OFF
```

开发版缺少某个协议库只影响该协议，连接时返回 `BACKEND_UNAVAILABLE`。开发版默认从 CLI 所在目录加载适配库；发行版默认自动释放内嵌库。两者均可通过绝对路径 `REMOTE_COMPUTER_LIB_DIR` 显式覆盖，使用 `remote-computer-ctl doctor --json` 检查。

## 配置

```sh
mkdir -p ~/.remote-computer-use
chmod 700 ~/.remote-computer-use
# 首次创建；已有配置时直接编辑，不覆盖。
(umask 077; set -C; cat config.example.toml > ~/.remote-computer-use/config.toml)
chmod 600 ~/.remote-computer-use/config.toml
```

编辑 [配置示例](config.example.toml) 中的主机、帐号和非敏感连接别名。密码可写入配置，也可用 `password_env` 指定环境变量名；二者互斥。`password_env` 在连接时读取，变量必须存在于启动 ctl 的环境中。查询连接不读取密码变量。配置变更须断开后重新连接才生效。

配置权限不允许其他用户读取；未知字段、无效端口和无效别名都会返回 `CONFIG_INVALID`，不回显配置内容。默认文件为 `~/.remote-computer-use/config.toml`，可用 `remote-computer-ctl --config`（位于子命令后）或 `REMOTE_COMPUTER_CONFIG` 覆盖。

连接安全策略：

- RDP 使用 TLS / NLA，默认验证系统 CA 和主机名；可设置 `certificate_sha256` 固定证书。指纹须通过可信渠道取得，不自动接受未知证书。固定指纹替代 CA / 主机名验证。
- 对于常见自签名 RDP 服务，可显式设置 `tls_verify = false` 跳过证书校验，仍使用 TLS 加密，但不验证服务端身份。它与 `certificate_sha256` 互斥，仅适用于 RDP；省略时默认为 `true`，不会在失败后自动跳过校验。
- Windows 自签名证书的只读指纹获取步骤见 [Windows RDP 证书指纹](docs/windows-certificate.md)。
- VNC 优先选择 VeNCrypt X509；传统 VNC/ARD/无认证连接须显式设置 `allow_unencrypted = true`。同时提供传统认证但 VeNCrypt 不含受支持的 X509 子类型时，仅在该开关开启后重连尝试一次传统认证；证书或密码失败不触发降级。不自动选择匿名 TLS。目前 VNC 不支持 `certificate_sha256`，设置后返回 `UNSUPPORTED`。
- 查询、错误和日志只输出别名、协议、状态、画面尺寸等公开字段，不输出主机、端口、帐号、密码或证书信息。配置和进程内存仍属于本机私有数据。

## 命令

```sh
remote-computer-ctl list --json
remote-computer-ctl connect desktop --json
remote-computer-ctl status desktop --json
remote-computer-ctl disconnect desktop
remote-computer-ctl connect --all --json
remote-computer-ctl disconnect --all --json
```

`connect` 默认等待最多 30 秒，直到认证和初始画面接收完成；可用 `--timeout 60s` 调整。重复连接就绪会话、断开已停止会话均为幂等操作。连接全部最多并发 4 个，失败不回滚成功连接。断开全部包含已从配置删除但仍在运行的连接。断开不主动注销远端用户，服务端仍可按自身策略结束会话。

```sh
remote-computer-use --conn desktop screenshot --output screen.png --json
remote-computer-use --conn desktop screenshot --output - > screen.png
remote-computer-use --conn desktop move --x 300 --y 200
remote-computer-use --conn desktop click --x 300 --y 200 --button left --count 2
remote-computer-use --conn desktop scroll --dy 3
remote-computer-use --conn desktop drag --from 100,200 --to 500,300 --duration 500ms
remote-computer-use --conn desktop key CTRL+L
remote-computer-use --conn desktop type --text 'Hello'
remote-computer-use --conn desktop type --stdin < local-text.txt
```

每次操作必须显式指定 `--conn`，不会自动连接。通用参数为 `--json`、`--timeout`（默认 10 秒）、`--geometry-version`。截图返回的 `geometry_version` 可用于后续坐标操作，桌面尺寸变化时旧版本返回 `GEOMETRY_CHANGED`，应重新截图计算坐标。

坐标使用原始 PNG 的像素，左上角为 `(0,0)`，不按缩放后的预览坐标直接点击。滚轮 `dy` 正值向下、`dx` 正值向右，单位为刻度，实际移动距离由服务器和应用决定。Apple ARD 服务的水平滚动暂返回 `UNSUPPORTED`，避免被解释为点击；Windows 实测单轴方向正常，同时指定两轴在 Edge 中尚不稳定，暂分两次调用。截图不额外叠加光标，文件在本机生成；`--output -` 为纯 PNG 流，与 `--json` 互斥。

Windows 需要两个方向滚动时分别调用，并在需要时截图确认：

```sh
remote-computer-use --conn windows scroll --dx 3
remote-computer-use --conn windows scroll --dy 3
```

组合键如 `CTRL+SHIFT+S`、`ALT+F4`、`CMD+L`、`WIN+R`、`ENTER`、`TAB`。按顺序按下、逆序释放；拖拽或组合键中途取消会尽力释放按下的键和按钮。

`0.1.1` 新增跨命令保持输入（`0.1.0` 包不包含此功能）。要验证“按住鼠标拖动时 Esc”，使用：

```sh
remote-computer-use --conn desktop pointer-down --x 100 --y 200 --button left
remote-computer-use --conn desktop move --x 500 --y 300
remote-computer-use --conn desktop key ESC
remote-computer-use --conn desktop pointer-up --button left
```

`pointer-up` 在当前坐标释放指定按钮；`move` 保持已按下的按钮。`key-down SHIFT` / `key-up SHIFT` 可在多条操作之间保持修饰键，每条命令只接受一个键名，支持与 `key` 相同的键名和别名。`key` 只自动释放本次新按下的键，不松开之前显式保持的键。重复 down 不重复按键；`release-input` 释放该连接由本工具跟踪的所有键和按钮，可重复调用，不受旧尺寸版本限制。

脚本应在 `finally` 或 shell `trap` 中调用 `release-input`，不要只依赖正常流程的 up。保持状态由同连接的调用者共享，多条命令组成的流程并不具备事务隔离。后台在 **30 秒无成功输入**、尺寸变化、操作失败、请求取消及正常断开时尽力释放；截图和查询不续期。CLI 参数校验失败没有发送请求，不会触发后台立即释放。进程被强制杀死或传输中断时无法保证释放送达。

鼠标保持期间，完整 `click`、`drag` 和 `scroll` 返回 `INVALID_ARGUMENT`，防止隐式松开原有按钮；请使用 `move` 与显式 up。显式按键尚未释放时不允许 `type`。从 `0.1.0` 升级后须断开并重新连接，旧 worker 不会自动升级。

文本采用协议键事件：RDP Unicode / VNC keysym，实际字符支持受远端服务器、输入法和应用影响。实测 Linux、Windows、Mac 均可输入中文和 emoji；识别到 Apple ARD 服务时，非 BMP 字符自动拆成 UTF-16 代理对发送，普通 VNC 保持完整码点。命令用法不变，无需修改远端输入法或剪贴板。首期只支持 `--method keys`，剪贴板方法返回 `UNSUPPORTED`，没有静默降级。敏感文本可从私有输入流传入 `--stdin`，避免放入命令行和终端历史。

静默表示**本机不显示窗口**；远端用户可能看到鼠标和应用变化。桌面登录、锁屏、并发会话由远端服务决定。操作返回成功只代表输入已发送，AI 必须再次截图确认应用结果。切换窗口、打开应用或点击输入框后，先等待并截图确认焦点，再输入文本；启用输入法时，ASCII 也可能被组合转换。超时或断线前可能已执行部分输入，不应盲目重试。

## JSON 与错误

`list` 返回公开连接信息数组；`status` 返回单项信息；连接、断开和桌面操作返回 `{ "ok": true/false, ... }`。批量操作返回结果数组。命令成功退出 0，失败退出 1；批量中任一失败即退出 1。`status` 仅在 `ready` 时退出 0。

截图写入文件后的响应示例：

```json
{"ok":true,"width":1600,"height":900,"geometry_version":1,"frame_sequence":4,"output":"screen.png"}
```

| 错误码 | 含义及处理 |
| --- | --- |
| `CONFIG_INVALID` | 检查私有配置格式、权限或缺失的密码变量 |
| `BACKEND_UNAVAILABLE` | 检查适配库及其运行依赖是否安装 |
| `CONNECT_FAILED` / `AUTH_FAILED` / `TLS_VERIFY_FAILED` | 分别检查服务、认证、证书；不泄露底层错误原文 |
| `NOT_CONNECTED` / `NOT_READY` | 先连接，或等待新完整画面 |
| `INVALID_ARGUMENT` / `UNSUPPORTED` | 参数无效、坐标越界或能力未提供 |
| `GEOMETRY_CHANGED` | 重新截图并使用新坐标版本 |
| `CONNECTION_LOST` / `TIMEOUT` / `CANCELED` | 检查连接和当前画面后再决定是否继续 |
| `LOCAL_IO_FAILED` / `OPERATION_FAILED` | 本地文件或内部操作失败 |

运行目录为 `$XDG_RUNTIME_DIR/remote-computer-use`，未配置时使用 `/tmp/remote-computer-use-UID`，权限 0700。`REMOTE_COMPUTER_RUN_DIR` 可指定隔离目录；所有 CLI 必须使用同一目录。Windows 默认使用 `%LOCALAPPDATA%/remote-computer-use/run`，通过 DACL 保护。CLI 与 worker 使用带随机令牌的本机 HTTP，运行目录不再受 socket 路径长度限制；见 [HTTP 接口与转发规划](docs/http-ipc.md)。

## AI Agent Skill

共用 [remote-computer-use Skill](skills/remote-computer-use/SKILL.md)，不需要额外的 MCP 服务或三套不同命令协议。

```sh
python3 scripts/install-skill.py --agent codex
# 按需选择其他 Agent：
python3 scripts/install-skill.py --agent opencode
python3 scripts/install-skill.py --agent claude-code
```

对应目录为 [Codex 的 `~/.agents/skills`](https://learn.chatgpt.com/docs/build-skills)、[OpenCode 的 `~/.config/opencode/skills`](https://opencode.ai/docs/skills)、[Claude Code 的 `~/.claude/skills`](https://code.claude.com/docs/en/skills)。OpenCode 的目录遵循 `XDG_CONFIG_HOME`。安装器也支持 `--dest /path/to/skills`，已有不同内容时需显式 `--force` 更新，不修改 Agent 的其他设置。

确保 Agent 运行环境的 PATH 能找到两个 CLI，并能访问所需的本机文件和网络。安装目录验证和 Skill 格式校验已经完成；三个 Agent 的实际自动发现与执行仍待各客户端联调。

## 测试

```sh
make test                    # Go 单元测试、race detector、go vet
sudo apt-get install -y tigervnc-standalone-server tigervnc-tools \
  x11-xserver-utils python3-pil libx11-dev libxext-dev libxrandr-dev \
  libxfixes-dev libxdamage-dev libxcursor-dev libxi-dev libxinerama-dev \
  libxkbfile-dev libxtst-dev

scripts/build-freerdp.sh --with-test-server
PKG_CONFIG_PATH="$PWD/.cache/freerdp/lib/pkgconfig" make native
make integration-rdp-test    # 同时验证真实 VNC 和 RDP
```

仅 VNC 可运行 `make integration-test`。测试启动临时回环桌面，临时凭据仅用于测试；CLI 进程的 DISPLAY 和 WAYLAND_DISPLAY 均被移除。测试专用桌面与 RDP shadow 需要 X11，产品运行时不需要。没有使用或连接用户的远端机器。
