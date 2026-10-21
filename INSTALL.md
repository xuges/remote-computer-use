# remote-computer-use 安装指南（面向 AI Agent）

本文件供具备本机 shell 权限的 AI Agent 执行。安装目标不是只复制一份 `SKILL.md`，而是让技能真正可用：

1. 安装 `remote-computer-ctl` 和 `remote-computer-use` 两个命令行工具，并让它们能从 Agent 的运行环境访问。
2. 把 `remote-computer-use` Skill 安装到当前 Agent 的用户级技能目录。
3. 创建私有配置模板，确认本机运行库可用，并完成自检。
4. 只有在用户提供真实连接信息后，才配置并连接远端桌面。

本技能只负责远端 GUI 操作。远程命令和文件传输属于 `remote-shell`，本安装不包含它。本项目不需要 MCP 服务，也不要把它改造成 MCP 服务。更完整的发行包、缓存和安全说明见 `README.md` 与 `docs/installation.md`。

项目仓库地址：<https://github.com/xuges/remote-computer-use>

如果本机还没有源码，先克隆并进入仓库：

```sh
git clone https://github.com/xuges/remote-computer-use.git
cd remote-computer-use
```

只用发行包安装时不需要克隆仓库；源码构建和使用 `scripts/install-skill.py` 时必须先取得仓库。

## 执行原则

AI 安装时必须遵守以下约束：

- 使用普通用户权限安装，不使用 `sudo`、管理员权限或系统级目录。
- 只写入用户目录和 Agent 的技能目录；不修改 Agent 的模型、权限、MCP、其他 Skill 或系统安全策略。
- 不修改 shell 启动文件、PowerShell profile 或系统 PATH，除非用户明确同意并知道影响范围。
- 不输出、转述或写入日志：配置内容、主机名、帐号、密码、证书指纹、端点令牌。
- 不猜测远端地址、帐号或密码，不尝试暴力认证。
- 不为了让连接成功而自动关闭 TLS 校验、启用未加密 VNC 或修改 Defender/Gatekeeper。
- 不覆盖未知的已有命令或内容不同的 Skill；发生冲突时停止并让用户决定。
- 不要在安装阶段执行 `connect --all`、`disconnect --all` 或连接配置模板中的示例主机；这些操作会影响用户已有会话。
- 不要翻译或改写 `SKILL.md`。安装器会用文件内容检测冲突。

安装本身不需要管理员权限。源码构建可能需要安装系统开发包；如果缺少这些依赖，应先征求用户同意，不要擅自使用 `sudo`。

## 一、确认平台

发行包支持以下**运行 CLI 的本机平台**：

| 平台 | 最低要求 | 发行包 |
| --- | --- | --- |
| Linux x64 | glibc 2.35+ | `linux-amd64.tar.gz` |
| Linux arm64 | glibc 2.35+，仅完成 QEMU 验证 | `linux-arm64.tar.gz` |
| macOS arm64 | macOS 15.0+ | `darwin-arm64.tar.gz` |
| Windows x64 | 已在 Windows 10 22H2 验证 | `windows-amd64.zip` |

Linux 上先检查架构和 glibc：

```sh
uname -s
uname -m
ldd --version | sed -n '1p'
```

macOS 检查架构：

```sh
uname -s
uname -m
```

Windows 检查系统架构：

```powershell
[Environment]::Is64BitOperatingSystem
$env:PROCESSOR_ARCHITECTURE
```

遇到以下情况应停止，并向用户说明不支持，不要尝试绕过：

- Linux 使用 musl/Alpine，或 glibc 低于 2.35。
- macOS 是 Intel 或低于 15.0。
- Windows 是 ARM64、32 位或非 x64。
- 远端只有 SSH，没有启用 VNC 或 RDP。本工具不是 SSH 客户端。

平台风险也需要如实告知用户：

- macOS 发行包未完成 Developer ID 签名和 Apple 公证。不要自动执行 `xattr` 移除隔离属性，也不要关闭 Gatekeeper。
- Windows EXE 未签名，可能被 Defender ASR 拦截。不要关闭 Defender、添加排除项或修改策略；只报告拦截现象并建议由用户或管理员处理。
- Linux arm64 仅完成 QEMU 验证，尚待实机验收。

## 二、识别当前 Agent

先判断 Skill 应安装到哪里。不能确定时，询问用户当前使用的是哪个 Agent，或让用户给出技能根目录。

| Agent | 用户级技能目录 | 安装器参数 |
| --- | --- | --- |
| Codex | `~/.agents/skills/remote-computer-use` | `codex` |
| OpenCode | `${XDG_CONFIG_HOME:-~/.config}/opencode/skills/remote-computer-use` | `opencode` |
| Claude Code | `~/.claude/skills/remote-computer-use` | `claude-code` |
| 其他 Agent | 用户指定的 skills 根目录 | `--dest` 或手动复制 |

Windows 下这些路径位于 `%USERPROFILE%` 下。PowerShell 发行包安装器对 OpenCode 固定使用 `%USERPROFILE%\.config\opencode\skills`；如果目标环境依赖 `XDG_CONFIG_HOME`，使用源码安装器的 `--dest` 或手动复制。

默认安装为用户级技能。只有用户明确要求项目级技能时，才把 `SKILL.md` 放到项目的 skills 目录，并在报告中说明作用范围。

## 三、选择安装方式

| 场景 | 推荐方式 |
| --- | --- |
| 普通安装，本机没有构建环境 | 自包含发行包（推荐） |
| 已有仓库，且 Go、CMake、libgvnc、FreeRDP 等依赖齐全 | 源码构建 |
| 两个 CLI 已经可用，只缺 Skill | `install-skill.py` 或手动复制 |

无论使用哪种方式，CLI 和 `SKILL.md` 应来自同一版本或同一提交。不要把旧版 CLI 和新版 Skill 混用。

## 四、方式 A：安装自包含发行包

### 1. 获取并校验发行包

从 [GitHub Releases](https://github.com/xuges/remote-computer-use/releases) 获取与平台匹配的包和同名 `.sha256` 文件。不要猜测版本号或资产名；以 Releases 页面实际列出的资产为准。如果页面没有所需资产，不要拼接或猜测下载地址；让用户提供本地包，或在用户同意后使用方式 B 从源码构建。

如果环境中有已认证的 GitHub CLI，可使用实际 tag 和版本下载（替换示例变量）：

```sh
TAG=v0.1.0
VERSION=0.1.0
gh release download "$TAG" --repo xuges/remote-computer-use \
  --pattern "remote-computer-use-$VERSION-linux-amd64.tar.gz*"
```

如果用户已经提供本地包，直接使用该包，不要重新下载。

Linux：

```sh
VERSION=0.1.0
ASSET=remote-computer-use-${VERSION}-linux-amd64.tar.gz
sha256sum -c "${ASSET}.sha256"
tar -xzf "$ASSET"
cd "remote-computer-use-${VERSION}-linux-amd64"
```

macOS：

```sh
VERSION=0.1.0
ASSET=remote-computer-use-${VERSION}-darwin-arm64.tar.gz
shasum -a 256 -c "${ASSET}.sha256"
tar -xzf "$ASSET"
cd "remote-computer-use-${VERSION}-darwin-arm64"
```

Windows PowerShell：

```powershell
$Version = "0.1.0"
$Asset = "remote-computer-use-$Version-windows-amd64.zip"
$expected = (Get-Content "$Asset.sha256").Split()[0].ToLower()
$actual = (Get-FileHash -Algorithm SHA256 $Asset).Hash.ToLower()
if ($actual -ne $expected) { throw "SHA-256 mismatch" }
Expand-Archive -LiteralPath $Asset -DestinationPath .
Set-Location "remote-computer-use-$Version-windows-amd64"
```

校验失败时停止，不要继续解压或安装。校验通过后，必须在解压目录中执行安装器。

### 2. 安装 CLI 和 Skill

Linux/macOS，把 `AGENT` 的值替换为 `codex`、`opencode` 或 `claude-code`：

```sh
AGENT=opencode
./install.sh --agent "$AGENT"
```

Windows PowerShell：

```powershell
powershell -File .\install.ps1 -Agent opencode
```

安装器默认执行以下操作：

- 把两个 CLI 和运行库安装到 `~/.remote-computer-use`，Windows 为 `%USERPROFILE%\.remote-computer-use`。
- 在 `~/.local/bin` 创建命令入口；Windows 在 `%USERPROFILE%\.remote-computer-use\bin` 创建 `.cmd` 入口。
- 首次创建私有配置模板；已有配置保留。
- 使用指定 Agent 参数时，把发行包内的 `skills/remote-computer-use/SKILL.md` 复制到对应用户级技能目录。

需要自定义位置时：

```sh
./install.sh --prefix /absolute/path --bin-dir /absolute/path --agent opencode
```

```powershell
powershell -File .\install.ps1 -Prefix C:\Tools\remote-computer-use -Agent opencode
```

`--prefix` 和 `--bin-dir` 必须是绝对路径。安装器和已有命令、已有不同 Skill 冲突时会拒绝继续；不要删除或覆盖未知文件，先让用户确认。

### 3. 让 Agent 能找到命令

Linux/macOS：

```sh
export PATH="$HOME/.local/bin:$PATH"
command -v remote-computer-ctl
command -v remote-computer-use
```

如果入口不在 `~/.local/bin`，使用安装器实际打印的目录。也可以直接使用 `~/.remote-computer-use/bin/remote-computer-ctl` 和 `~/.remote-computer-use/bin/remote-computer-use`。

Windows PowerShell：

```powershell
$env:Path = "$HOME\.remote-computer-use\bin;$env:Path"
Get-Command remote-computer-ctl
Get-Command remote-computer-use
```

这里的 `export` 和 `$env:Path` 只对当前进程有效。为了后续 Agent 会话也能使用，用户需要把该目录加入其 shell 或 Agent 的持久环境。未经用户同意，不要修改 `.bashrc`、`.zshrc`、`.profile` 或 PowerShell profile。报告中应明确写出需要加入 PATH 的目录。

### 4. 非 Codex/OpenCode/Claude Code 的 Agent

发行包的 `install.sh` 只支持上述三个 `--agent` 值。其他 Agent 先安装 CLI，不带 `--agent`：

```sh
./install.sh
```

然后在解压目录中手动复制 Skill 到用户的技能根目录：

```sh
SKILLS_ROOT=/absolute/path/to/agent/skills
mkdir -p "$SKILLS_ROOT/remote-computer-use"
install -m 644 skills/remote-computer-use/SKILL.md "$SKILLS_ROOT/remote-computer-use/SKILL.md"
```

Windows PowerShell：

```powershell
$SkillsRoot = "C:\absolute\path\to\agent\skills"
New-Item -ItemType Directory -Force "$SkillsRoot\remote-computer-use" | Out-Null
Copy-Item .\skills\remote-computer-use\SKILL.md "$SkillsRoot\remote-computer-use\SKILL.md"
```

只复制 Skill 文件而不安装 CLI，不算完成安装。CLI 未安装时，Agent 虽然能发现技能，但执行操作会失败。

## 五、方式 B：从源码构建

仅在用户已有仓库并同意安装构建依赖时使用。如果还没有仓库，按本文件开头的命令从 `https://github.com/xuges/remote-computer-use.git` 克隆。构建要求：Go 1.25.8+、C 编译器、CMake、pkg-config、libgvnc 1.3+、FreeRDP 3.25+。

Ubuntu 示例（需要用户授权后才能使用 `sudo`）：

```sh
sudo apt-get update
sudo apt-get install -y build-essential cmake ninja-build pkg-config curl patchelf python3 \
  libgvnc-1.0-dev libglib2.0-dev libssl-dev libkrb5-dev zlib1g-dev \
  libjpeg-dev libpng-dev libicu-dev uuid-dev
```

如果发行版没有足够新的 FreeRDP，可构建固定版本（需要网络下载并校验源码）：

```sh
FREERDP_PREFIX="$HOME/.local/lib/remote-computer-use/freerdp" scripts/build-freerdp.sh
export PKG_CONFIG_PATH="$HOME/.local/lib/remote-computer-use/freerdp/lib/pkgconfig"
```

在仓库根目录构建和安装开发版：

```sh
make native
make build
make install
export PATH="$HOME/.remote-computer-use/bin:$PATH"
python3 scripts/install-skill.py --agent opencode
```

`make native` 需要能链接 libgvnc 和 FreeRDP。缺少原生库时，Go 二进制即使构建成功也不能连接桌面，`doctor` 会返回 `BACKEND_UNAVAILABLE`。开发版依赖构建时使用的原生库位置；需要可搬运的离线包时，使用方式 A 的发行包。

如果只需要一种协议，可按需构建：

```sh
make native NATIVE_FLAGS='-DRCU_BUILD_RDP=OFF -DRCU_BUILD_VNC=ON'
```

此时只验证实际构建的协议，并在报告中说明另一种协议不可用。

## 六、方式 C：只安装 Skill

仅当两个 CLI 已存在、版本正确，或用户明确只要求更新 Skill 时使用。如果还没有仓库，先执行本文件开头的 `git clone` 命令。

在仓库根目录：

```sh
python3 scripts/install-skill.py --agent opencode
```

自定义技能根目录：

```sh
python3 scripts/install-skill.py --dest /absolute/path/to/skills
```

`--dest` 会安装到 `/absolute/path/to/skills/remote-computer-use`。已有 `SKILL.md` 内容相同时安装器会报告已安装；内容不同时会拒绝。只有用户明确同意替换时才能使用 `--force`，并应先保留或备份原文件。

如果当前环境没有 Python，使用发行包内的 `install.sh --agent ...`，或按“非三种 Agent”的方法手动复制 `SKILL.md`。发行包安装本身不需要 Python。

## 七、配置远端连接

安装器首次会创建配置模板：

- Linux/macOS：`~/.remote-computer-use/config.toml`
- Windows：`%USERPROFILE%\.remote-computer-use\config.toml`

模板中的 `desktop` 和 `windows` 是示例，不是可用主机。不要在未替换占位信息时执行连接。

### 1. 检查私有权限

Linux：

```sh
stat -c '%a %n' ~/.remote-computer-use ~/.remote-computer-use/config.toml
```

macOS：

```sh
stat -f '%Lp %N' ~/.remote-computer-use ~/.remote-computer-use/config.toml
```

目录应为 `700`，配置文件应为 `600`。Windows 安装器会为新配置设置当前用户、SYSTEM 和 Administrators 的私有 DACL；不要把配置复制到共享目录。

### 2. 向用户收集必要信息

只向用户询问真正需要的内容：

- 连接别名，例如 `desktop`。
- 协议：`vnc` 或 `rdp`。
- 主机和端口。
- RDP 的用户名；域环境还需要域名。
- 可选的 RDP 分辨率 `width`、`height`。
- 凭据来源：推荐 `password_env`，也可以由用户选择 `password`。
- 是否需要证书固定 `certificate_sha256`，或是否明确接受 `tls_verify = false`。
- VNC 是否为传统未加密认证；如果是，是否明确接受 `allow_unencrypted = true`。

不要在对话中索要或回显密码。推荐由用户在自己的安全环境中设置环境变量，并在配置中使用变量名：

```toml
[connections.desktop]
protocol = "vnc"
host = "desktop.example.com"
port = 5900
password_env = "RCU_DESKTOP_PASSWORD"
allow_unencrypted = false
```

`password_env` 指定的变量必须在启动 `remote-computer-ctl` 的同一环境中存在。如果 Agent 的每条命令都使用新的 shell，应由用户的 Agent 启动环境或 secret 注入机制提供该变量，不要把它写进命令行历史、日志或对话。

### 3. 安全选项

- 默认保持 RDP `tls_verify = true`，验证系统 CA 和主机名。
- 自签名 RDP 优先使用可信渠道取得的 `certificate_sha256` 固定证书。
- `tls_verify = false` 仍使用 TLS 加密，但不验证服务端身份；只能在用户明确知情并同意时设置。
- VNC 传统认证或未加密传输默认拒绝；只有用户明确同意使用该传输时才设置 `allow_unencrypted = true`。
- 配置修改后，已有连接需要断开并重新连接才会生效。
- 配置权限过宽、未知字段、无效端口或无效别名会返回 `CONFIG_INVALID`；修复后重试，不要通过打印配置内容排查。

### 4. 验证配置但不连接

```sh
remote-computer-ctl list --json
```

该命令只读取公开别名和状态，不读取密码变量，也不访问远端。输出中不应出现主机、端口、帐号或密码。

## 八、安装验证

### 1. CLI 和运行库

```sh
remote-computer-ctl --version
remote-computer-use --version
remote-computer-ctl doctor --json
```

通过条件：

- 两个 `--version` 输出一致。
- 发行包安装时，`doctor --json` 顶层 `ok` 为 `true`，`checks` 中 VNC 和 RDP 的 `ok` 均为 `true`。
- 源码构建如果刻意省略一种协议，所需协议的 `checks[].ok` 必须为 `true`；整体 `ok` 可能为 `false`，应在报告中说明另一种协议不可用。
- 发行包安装时 `runtime` 通常为 `embedded`，表示使用内嵌运行库。

`doctor` 不读取配置，不访问远端，也不验证远端凭据。首次运行会离线释放内嵌运行库到私有缓存。

### 2. Skill 文件

确认目标文件存在，例如 OpenCode：

```sh
test -f "${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills/remote-computer-use/SKILL.md"
```

Codex：

```sh
test -f "$HOME/.agents/skills/remote-computer-use/SKILL.md"
```

Claude Code：

```sh
test -f "$HOME/.claude/skills/remote-computer-use/SKILL.md"
```

使用文件读取工具确认文件开头是 YAML frontmatter，并且包含：

```text
name: remote-computer-use
description: ...
```

如果从发行包安装，可与包内文件逐字节比较（在解压目录中执行）：

```sh
cmp -s skills/remote-computer-use/SKILL.md \
  "${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills/remote-computer-use/SKILL.md"
```

Windows PowerShell：

```powershell
$installed = "$HOME\.config\opencode\skills\remote-computer-use\SKILL.md"
(Get-FileHash .\skills\remote-computer-use\SKILL.md).Hash -eq (Get-FileHash $installed).Hash
```

不要创建指向未知位置的符号链接，也不要安装多份内容不同的同名 Skill。

### 3. 端到端验证（可选）

只有用户已经提供真实配置并明确同意连接时，才执行以下步骤。把 `ALIAS` 的值改为真实连接别名：

```sh
ALIAS=desktop
remote-computer-ctl list --json
remote-computer-ctl connect "$ALIAS" --json
remote-computer-use --conn "$ALIAS" screenshot --output /tmp/rcu-check.png --json
remote-computer-ctl disconnect "$ALIAS" --json
```

使用实际的 PNG 查看工具确认截图内容；不要只根据命令退出码判断连接成功。截图文件在本机生成，远端用户可能同时看到桌面操作。无论中间步骤是否成功，最后都应执行 `disconnect` 清理本次创建的连接。

如果用户没有提供连接信息，安装完成到 `doctor` 通过即可。应报告“已安装，配置待用户填写”，不要伪造连接成功。

## 九、故障处理

| 现象 | 处理 |
| --- | --- |
| `remote-computer-ctl: command not found` | 检查 PATH，或直接使用 `~/.remote-computer-use/bin/...`；Windows 使用 `.cmd` 入口。 |
| `BACKEND_UNAVAILABLE` | 检查平台/glibc、缓存目录是否可写且允许执行、是否错误设置了 `REMOTE_COMPUTER_LIB_DIR`。有活动连接时不要清理缓存。 |
| `CONFIG_INVALID` | 检查配置文件权限、未知字段、端口、别名和密码变量是否存在。不要打印配置内容。 |
| `AUTH_FAILED` / `TLS_VERIFY_FAILED` / `CONNECT_FAILED` | 让用户核对服务、凭据或证书。不要自动关闭 TLS 校验或启用未加密 VNC。 |
| `NOT_CONNECTED` / `NOT_READY` | 先执行 `connect`，或等待完整画面可用。 |
| Skill 未被 Agent 发现 | 确认 Agent 类型和技能目录；重启或刷新 Agent 会话；确认 frontmatter 未被改写。 |
| 安装器提示已有 Skill 内容不同 | 停止。比较两份文件，由用户决定保留、备份或替换；不要未经同意使用 `--force`。 |
| 安装器拒绝替换已有命令 | 停止。该命令可能属于其他软件，由用户决定如何处理。 |
| Windows 启动被 Defender/ASR 阻止 | 不要关闭防护或添加排除项。保留事件记录和文件哈希，报告给用户或管理员。 |
| macOS 被 Gatekeeper 阻止 | 不要自动移除隔离属性。向用户说明未公证状态，由用户决定是否继续。 |
| 源码安装缺少原生库 | 使用发行包，或在用户同意后安装构建依赖并重新执行 `make native`。 |

## 十、升级与卸载

升级时使用新发行包中的 `install.sh` 或 `install.ps1` 重新执行安装。安装器保留已有配置，并按内容哈希保留旧版本文件。已有连接不会自动切换到新 worker；升级后应断开并重新连接。

卸载只在用户明确要求时执行。先断开所有连接：

```sh
remote-computer-ctl disconnect --all --json
```

然后删除安装器创建的命令入口、`~/.remote-computer-use/releases`、`current`、`bin` 和项目缓存目录（Linux 为 `${XDG_CACHE_HOME:-$HOME/.cache}/remote-computer-use`，macOS 为 `~/Library/Caches/remote-computer-use`），以及选定 Agent 的 `remote-computer-use` 技能目录。不要删除 `config.toml`，除非用户明确要求删除连接配置。

Windows 删除 `%USERPROFILE%\.remote-computer-use` 下的版本目录和 `bin` 入口，以及 `%LOCALAPPDATA%\remote-computer-use` 缓存，并保留配置文件。

## 十一、安装完成后的报告格式

向用户报告时只包含公开信息，不包含地址、帐号、密码、证书内容或令牌。建议格式：

```text
状态：安装完成 / 已安装但待配置 / 安装失败
平台：
Agent 与技能目录：
安装方式与版本：
CLI 路径：
Skill 路径：
doctor 结果：
配置状态：模板已创建 / 已有配置保留 / 待用户提供真实连接信息
需要用户执行的下一步：
```

只有以下条件全部满足，才能报告“安装完成”：

- 两个 CLI 可从 Agent 运行环境解析，且版本一致。
- `doctor --json` 显示所需协议 `ok` 为 `true`；发行包应同时具备 VNC 和 RDP。
- `SKILL.md` 位于正确的用户级技能目录，内容与安装来源一致。
- 配置文件存在且权限私有；示例占位信息没有被误当作真实连接。
- 如果执行了远端连接验证，必须由用户提供目标并授权；没有验证时明确说明未验证。
