# 初版验证记录

## 已验证

环境：Ubuntu 22.04 x86_64、Go 1.25.8、libgvnc 1.3.0、FreeRDP 3.25.0。

| 层次 | 覆盖内容 |
| --- | --- |
| Go 单元测试与 race | 配置权限/未知字段/凭据互斥、私有错误边界、IPC 分帧与大数据、取消释放、组合键部分失败释放、坐标与尺寸版本、无关命令参数拒绝 |
| Go 静态检查 | `go vet ./...` |
| 构建与安装 | 固定归档校验、全新目录构建无 X11 的 FreeRDP、两种适配库动态加载、临时目录安装 CLI、三个 Agent 的 Skill 安装路径/幂等/覆盖保护与格式校验 |
| 真实 VNC 回环 | 密码认证、RGB PNG、增量画面、移动/双击/拖拽/滚轮/组合键/文本、真实尺寸变化、旧坐标拒绝、客户端退出释放鼠标 |
| 真实 RDP 回环 | TLS、固定证书指纹与错误指纹拒绝、NLA 正确/错误密码、RGB PNG、鼠标和键盘输入 |
| 生命周期 | 重复连接/断开、并行 VNC/RDP 会话、批量部分失败保留健康会话、服务端断开后操作失败、全部断开清理 socket |
| 脱敏与后台运行 | CLI 无 DISPLAY/WAYLAND_DISPLAY、输出不包含测试帐号和密码、公开状态不包含主机和端口 |

测试实现位于 `tests/integration.py` 与 `internal/*/*_test.go`。真实服务只绑定回环地址，使用临时配置和测试凭据，退出后清理。

## 待验证与暂不实现

- macOS 屏幕共享 / ARD、Windows RDP、Linux xrdp 实机兼容性。
- VNC VeNCrypt X509 的完整证书链与主机名互操作；RDP 企业 CA、域帐号与服务器会话策略。
- 非拉丁文字、emoji、键盘布局和输入法组合；目前协议级实现 Unicode，不能由 ASCII 测试推断全部应用兼容。
- 长时间运行、网络抖动、大分辨率负载和原生库 sanitizer 测试；Go race detector 不覆盖 C 层。
- 原生进程崩溃恢复、RDP 动态分辨率以及物理多显示器的专项验收。
- 本机 macOS / Windows 移植、剪贴板输入、视频展示、音频、文件/设备重定向不在本次实现范围。
- Agent Skill 格式和安装路径已验证；Codex、OpenCode、Claude Code 的完整端到端联调未完成。

兼容性结论限于已运行的测试，不能将 Linux shadow 回环等同于 Windows 实机验证。
