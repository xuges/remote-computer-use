# remote-computer-use

让 AI 在本机通过 VNC / RDP 观察和操作远端桌面。两个 Go 命令行工具共用后台会话：

- `remote-computer-ctl`：查询、连接、断开，以及连接全部、断开全部。
- `remote-computer-use`：截图、移动、点击、拖拽、滚轮、组合键和文本输入。

与 [remote-shell](https://github.com/xuges/remote-shell) 独立互补：remote-shell 处理远程命令和文件，本项目处理 GUI。远端只需提供 VNC / RDP 服务，无需安装本项目的执行助手。本机不打开窗口，不依赖 DISPLAY、Wayland、Xvfb 或外部键鼠工具；截图来自协议解码后的内存画面。

目前是首个可运行版本，本机支持 Linux，已验证 Ubuntu 22.04 x86_64。远端 macOS、Windows 和 Linux 的覆盖目标依赖各自 VNC / RDP 服务；自动化集成测试使用 TigerVNC 和 FreeRDP shadow。**Mac VNC、Windows RDP 实机均已验证连接与截图，实机键鼠尚未验证。** 详细架构与后续计划见 [设计文档](docs/design.md)，测试边界见 [验证记录](docs/validation.md)。

## 构建

要求 Go 1.25.8 或更新版本、C 编译器、CMake、pkg-config、libgvnc ≥ 1.3、FreeRDP ≥ 3.25。Go 使用 `github.com/goexlib/cgo` 动态加载 C 适配库，两个 CLI 使用 `CGO_ENABLED=0` 构建。**Go 二进制仍需要相应协议的原生共享库。**

Ubuntu 22.04 示例：

```sh
sudo apt-get update
sudo apt-get install -y build-essential cmake ninja-build pkg-config curl \
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

构建脚本校验下载归档的 SHA-256，不执行系统安装。`make install` 将两个 CLI 和已构建的适配库复制到 `~/.remote-computer-use/bin`，可通过 `PREFIX=/absolute/path` 更改；不会创建或覆盖连接配置。FreeRDP、libgvnc 等运行库仍须保留在原安装位置，当前没有可跨机器搬运的依赖打包发行物。

开发时可直接使用 `bin/`。仅构建一种协议：

```sh
make native NATIVE_FLAGS='-DRCU_BUILD_RDP=OFF -DRCU_BUILD_VNC=ON'
# 或 -DRCU_BUILD_RDP=ON -DRCU_BUILD_VNC=OFF
```

缺少某个协议库只影响该协议，连接时返回 `BACKEND_UNAVAILABLE`。适配库默认从 CLI 所在目录加载，也可通过绝对路径 `REMOTE_COMPUTER_LIB_DIR` 指定。

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

坐标使用原始 PNG 的像素，左上角为 `(0,0)`，不按缩放后的预览坐标直接点击。滚轮 `dy` 正值向下、`dx` 正值向右，单位为刻度。截图不额外叠加光标，文件在本机生成；`--output -` 为纯 PNG 流，与 `--json` 互斥。

组合键如 `CTRL+SHIFT+S`、`ALT+F4`、`CMD+L`、`WIN+R`、`ENTER`、`TAB`。按顺序按下、逆序释放；拖拽或组合键中途取消会尽力释放本次按下的键和按钮。首期不公开跨命令持续按住的接口。

文本采用协议键事件：RDP Unicode / VNC keysym，实际字符支持受远端服务器和应用影响。首期只支持 `--method keys`，剪贴板方法返回 `UNSUPPORTED`，没有静默降级。敏感文本可从私有输入流传入 `--stdin`，避免放入命令行和终端历史。

静默表示**本机不显示窗口**；远端用户可能看到鼠标和应用变化。桌面登录、锁屏、并发会话由远端服务决定。操作返回成功只代表输入已发送，AI 必须再次截图确认应用结果。超时或断线前可能已执行部分输入，不应盲目重试。

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

运行目录为 `$XDG_RUNTIME_DIR/remote-computer-use`，未配置时使用 `/tmp/remote-computer-use-UID`，权限 0700。`REMOTE_COMPUTER_RUN_DIR` 可指定隔离目录；所有 CLI 必须使用同一目录。Unix socket 路径长度有限，自定义运行目录最多 58 字节。

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
