# Changelog

## 0.1.1

- 新增 `pointer-down/up`、`key-down/up`、`release-input`，可跨命令组合拖动、修饰键和 Esc。
- `move` 保留已按下的鼠标按钮，`key` 不释放此前显式保持的键。
- 后台跟踪保持状态，空闲 30 秒、尺寸变化、操作失败或正常断开时尽力释放；升级后须重连旧会话。

## 0.1.0

首个版本，提供通过 VNC / RDP 静默操作远端桌面的统一命令行接口。

- `remote-computer-ctl`：查询、连接、断开、连接全部与断开全部，输出不暴露连接凭据。
- `remote-computer-use`：PNG 截图、移动、点击、拖拽、滚轮、组合键及文本输入，支持中文与 emoji。
- 后台 worker 保持桌面连接，通过带随机令牌的本机 HTTP 接受命令。
- 内嵌协议运行库，离线安装并保留已有配置，支持 Windows x64、Linux x64/arm64、macOS arm64。
- 共用 Agent Skill，提供 Codex、OpenCode、Claude Code 的安装入口。
- 项目使用 MIT；保留第三方许可，随版本提供对应源码包。Windows 移除不需要的 TIFF/JBIG 依赖链。

macOS 尚未完成 Developer ID 签名与公证；Windows 未签名 EXE 可能被 Defender ASR 拦截。Linux ARM64 尚无实机验收，部分滚动与 RDP 场景有限制。完整说明见 [v0.1.0 发布说明](docs/releases/v0.1.0.md)。
