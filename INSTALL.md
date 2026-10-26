# remote-computer-use 安装指南（面向 AI Agent）

安装只需要三步：

1. 从 GitHub Releases 下载对应平台的二进制发行包并校验。
2. 解压后把 `bin/` 复制到 `~/.remote-computer-use/bin`。
3. 把 `skills/remote-computer-use/SKILL.md` 复制到当前 Agent 的技能目录。

不需要克隆仓库，不需要 Go，不需要编译。发行包已经内嵌运行库。

## 1. 选择发行包

从 [GitHub Releases](https://github.com/xuges/remote-computer-use/releases) 下载与当前平台匹配的包和同名 `.sha256` 文件。不要猜测版本号或资产名。

| 平台 | 最低要求 | 发行包 |
| --- | --- | --- |
| Linux x64 | glibc 2.35+ | `linux-amd64.tar.gz` |
| Linux arm64 | glibc 2.35+，仅完成 QEMU 验证 | `linux-arm64.tar.gz` |
| macOS arm64 | macOS 15.0+ | `darwin-arm64.tar.gz` |
| Windows x64 | 已在 Windows 10 22H2 验证 | `windows-amd64.zip` |

不支持 Alpine/musl、macOS Intel、Windows ARM64/32 位。macOS 包未公证，Windows EXE 未签名；不要移除隔离属性、关闭 Gatekeeper/Defender 或添加排除项。

## 2. 下载、校验并解压

Linux：

```sh
VERSION=0.1.1
ASSET=remote-computer-use-${VERSION}-linux-amd64.tar.gz
sha256sum -c "${ASSET}.sha256"
tar -xzf "$ASSET"
cd "remote-computer-use-${VERSION}-linux-amd64"
```

macOS：

```sh
VERSION=0.1.1
ASSET=remote-computer-use-${VERSION}-darwin-arm64.tar.gz
shasum -a 256 -c "${ASSET}.sha256"
tar -xzf "$ASSET"
cd "remote-computer-use-${VERSION}-darwin-arm64"
```

Windows PowerShell：

```powershell
$Version = "0.1.1"
$Asset = "remote-computer-use-$Version-windows-amd64.zip"
$expected = (Get-Content "$Asset.sha256").Split()[0].ToLower()
$actual = (Get-FileHash -Algorithm SHA256 $Asset).Hash.ToLower()
if ($actual -ne $expected) { throw "SHA-256 mismatch" }
Expand-Archive -LiteralPath $Asset -DestinationPath .
Set-Location "remote-computer-use-$Version-windows-amd64"
```

校验失败时停止。后续命令都在解压目录中执行。

## 3. 复制 bin 和 SKILL.md

先确认当前 Agent 的技能目录：

| Agent | 技能目录 |
| --- | --- |
| Codex | `~/.agents/skills/remote-computer-use` |
| OpenCode | `${XDG_CONFIG_HOME:-~/.config}/opencode/skills/remote-computer-use` |
| Claude Code | `~/.claude/skills/remote-computer-use` |
| 其他 Agent | 用户指定的 skills 根目录下的 `remote-computer-use` |

Linux/macOS 示例（OpenCode）：

```sh
BIN_DIR="$HOME/.remote-computer-use/bin"
SKILL_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills/remote-computer-use"
mkdir -p "$BIN_DIR" "$SKILL_DIR"
cp -R bin/. "$BIN_DIR/"
cp skills/remote-computer-use/SKILL.md "$SKILL_DIR/"
chmod 755 "$BIN_DIR/remote-computer-ctl" "$BIN_DIR/remote-computer-use"
```

Codex 或 Claude Code 只需替换 `SKILL_DIR`：

```sh
SKILL_DIR="$HOME/.agents/skills/remote-computer-use"
SKILL_DIR="$HOME/.claude/skills/remote-computer-use"
```

Windows PowerShell 示例（OpenCode）：

```powershell
$Bin = "$HOME\.remote-computer-use\bin"
$Skill = "$HOME\.config\opencode\skills\remote-computer-use"
New-Item -ItemType Directory -Force $Bin, $Skill | Out-Null
Copy-Item .\bin\* $Bin -Recurse -Force
Copy-Item .\skills\remote-computer-use\SKILL.md $Skill -Force
```

如果已有内容不同的 `SKILL.md`，停止并让用户决定是否替换；不要直接覆盖。

## 4. 验证

```sh
"$HOME/.remote-computer-use/bin/remote-computer-ctl" --version
"$HOME/.remote-computer-use/bin/remote-computer-use" --version
"$HOME/.remote-computer-use/bin/remote-computer-ctl" doctor --json
```

通过条件：

- 两个 `--version` 输出一致。
- `doctor --json` 顶层 `ok` 为 `true`，VNC 和 RDP 的 `checks` 均为 `ok: true`。
- Skill 文件存在且内容与解压包一致：

```sh
SKILL_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills/remote-computer-use"
test -f "$SKILL_DIR/SKILL.md"
cmp -s skills/remote-computer-use/SKILL.md "$SKILL_DIR/SKILL.md"
```

为了让 Agent 后续直接调用命令，建议把 `BIN_DIR` 加入 PATH：

```sh
export PATH="$HOME/.remote-computer-use/bin:$PATH"
```

Windows：

```powershell
$env:Path = "$HOME\.remote-computer-use\bin;$env:Path"
```

只对当前进程设置 PATH 是安全的；持久化修改 shell 启动文件或系统 PATH 前应先征得用户同意。

## 5. 配置连接

手动复制不会自动创建配置模板。需要连接远端时，复制示例配置并设置私有权限：

```sh
cp config.example.toml "$HOME/.remote-computer-use/config.toml"
chmod 600 "$HOME/.remote-computer-use/config.toml"
```

Windows 复制到 `%USERPROFILE%\.remote-computer-use\config.toml`，并确保只有当前用户可读。

模板中的 `desktop` 和 `windows` 是示例。向用户询问真实别名、协议、主机、端口、RDP 用户名和凭据来源后再修改配置。推荐使用 `password_env`，不要把密码写入对话、命令行或日志。

安全默认值：

- 保持 RDP `tls_verify = true`；自签名证书优先使用 `certificate_sha256` 固定。
- 只有用户明确同意时才能设置 `tls_verify = false` 或 `allow_unencrypted = true`。
- 不猜测地址、帐号或密码，不尝试暴力认证。

配置完成后可以检查公开别名：

```sh
"$HOME/.remote-computer-use/bin/remote-computer-ctl" list --json
```

只有在用户提供真实配置并明确同意连接时，才执行 `connect` 和截图验证。

## 6. 常见问题

| 现象 | 处理 |
| --- | --- |
| `command not found` | 使用 `~/.remote-computer-use/bin/` 下的绝对路径，或把该目录加入 PATH。 |
| `BACKEND_UNAVAILABLE` | 检查平台/glibc、缓存目录是否可写且允许执行。有活动连接时不要清理缓存。 |
| `CONFIG_INVALID` | 检查配置权限、字段、端口、别名和密码变量；不要打印配置内容。 |
| `AUTH_FAILED` / `TLS_VERIFY_FAILED` / `CONNECT_FAILED` | 让用户核对服务和凭据；不要自动降低 TLS 或启用未加密 VNC。 |
| Skill 未被 Agent 发现 | 确认 Agent 类型和技能目录，重启或刷新 Agent 会话。 |

## 7. 完成标准

满足以下条件才算安装完成：

- 两个 CLI 已复制到 `~/.remote-computer-use/bin`，版本一致。
- `doctor --json` 显示所需协议 `ok: true`。
- `SKILL.md` 已复制到当前 Agent 的技能目录，内容与发行包一致。
- 未连接远端，或仅在用户授权后完成连接验证。

报告时不要包含主机、帐号、密码、证书内容或令牌。
