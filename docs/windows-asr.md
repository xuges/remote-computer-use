# Windows 启动拦截分析

2026-09-30，在同一 Windows x64 机器上复查 `dev` 发行包。结论：已确认 Defender ASR 拦截，但尚未确定内部判定因素；现有证据不支持将其归因于协议 DLL 加载失败。Windows 本机的完整 VNC 隔离测试本轮通过，新解压的 `dev` 包安装测试仍被拦截。

后续 `0.1.0` 版本包的完整安装与 VNC 复测通过，详见 [首版验证记录](validation.md#首版-010-产物验证)。版本号重编译改变了 EXE 内容，不能据此认定旧包触发原因已解决，也不能保证其他机器或后续下载不再被拦截。以下保留 `dev` 包诊断时的原始观察。

## 观察结果

所有时间为 UTC；未重新构建 EXE，未修改 Defender 策略、排除项或文件权限。

| 检查 | 结果 |
| --- | --- |
| 已有解压目录，PowerShell 启动 `use.exe --version` | 04:28:52 被拒绝，事件 1121 |
| 同目录，Go 测试程序启动 CLI | 完整 VNC 测试通过 |
| 同目录，cmd 启动 `use.exe --version` | 返回 `dev` |
| 同目录，PowerShell 再次启动同一 EXE | 04:33:14 返回 `dev`，原完整检查脚本随后也通过 |
| 从同一发行包新解压到临时目录，执行安装测试 | 04:34:11 在 `use.exe --version` 被拒绝，新增事件 1121 |
| 原生库自检 `ctl doctor` | 上述检查中均通过，包括新解压包 |
| Authenticode / 下载隔离标记 | 已有目录的两个 EXE 均为 NotSigned，无 Zone.Identifier |

VNC 测试使用 Windows 本机 CLI、原生 VNC 库、HTTP worker，以及独立的内存 RFB 服务。验证了连接、640×480 PNG 与颜色、中文和 emoji 键事件、键鼠、拖动取消后的按键释放、重连及长中文路径。它不操作用户桌面，也不等同于真实 VNC 服务的全部认证兼容性验收。本轮未继续测试 RDP 连接；doctor 只检查两个协议的库能否加载。

已有目录 `remote-computer-use.exe` 的 SHA-256 在拒绝和恢复期间保持为 `3d3f5071b6708833d55aa5f603d06edceb3cb50a9c2580476a586666c13f5de1`，与发行包一致。

## 已确认的拦截机制

Defender Operational 日志中的事件 1121 指向 `remote-computer-use.exe`，记录进程为 `powershell.exe`，规则为 `C1DB55AB-C21A-4637-BB3F-A12568109D35`，即 [Use advanced protection against ransomware](https://learn.microsoft.com/en-us/defender-endpoint/attack-surface-reduction-rules-reference#use-advanced-protection-against-ransomware)。

微软说明该规则使用本机和云端启发式判断，也会阻止尚未建立正面信誉的未知文件。文件有效签名、云端判定及普及程度会影响处理。事件本身不意味着已经确认程序包含勒索软件。

## 是否由内嵌 DLL 导致

当前证据不支持“释放或加载 DLL 导致这次拒绝”：

- [`Use` 的 `--version` 分支](../internal/app/cli.go)直接打印版本并返回，不调用协议库释放和加载。
- 真正执行运行库释放与加载的 [`ctl doctor`](../internal/app/doctor.go)持续通过；[`backend.Check`](../internal/backend/native.go)会实际加载适配库及其依赖。
- 完整 VNC 测试使用同一发行包并已通过，说明该机器能加载并运行这组 VNC DLL。
- 事件目标是 use EXE，没有提供某个 DLL 被拒绝的证据。

这些检查不能排除安全软件静态分析文件内容或综合行为产生的影响；但没有必要仅凭本次事件拆除 `go:embed`、改成静态链接或更换协议库。

## 原因判断与后续方向

优先调查新构建、未签名程序缺少信誉，以及新文件落地、启动上下文和判定缓存等因素。这是基于规则说明和实验现象的推断，日志没有给出具体评分依据。

不能单独归因于 PowerShell 或 `-EncodedCommand`：同一 PowerShell 检查后来恢复成功；也不能声称云端信誉已恢复，因为新解压包仍被同一规则阻止。目录、新文件创建时间、启动链和时间变化未完全隔离。

保留二进制哈希、事件时间及最小复现，以便进一步分析或向微软提交误拦截样本。正式代码签名和稳定的发行产物可作为改善方向，效果仍需复测。当前没有提交样本、添加排除项或更改系统防护策略，也不将其他启动方式固化成绕过拦截的安装逻辑。
