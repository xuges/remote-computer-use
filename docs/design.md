# remote-computer-use v1 设计

## 定位

提供两个 Go CLI：`remote-computer-ctl` 管理连接，`remote-computer-use` 提供统一截图和键鼠操作。使用 VNC/RDP 解码到内存并直接发送输入，不需要显示窗口、Xvfb、SSH 或远端执行助手。与 `remote-shell` 独立互补：本项目操作 GUI，remote-shell 执行远端命令和传输文件。

首期本机支持 Linux。远端需要已启用 VNC/RDP 服务。静默指本机无窗口，远端用户仍可能看到鼠标、输入和应用变化。服务端支持独立会话时使用独立会话，否则使用现有桌面；本工具不能阻止远端物理键鼠输入。

## 架构

每个连接使用一个后台进程，复用 ctl 二进制的内部运行模式。CLI 通过当前用户私有 Unix socket 请求操作。同一连接内输入动作串行，不同连接并行。协议事件循环持续解码，截图读取一致的完整内存快照，由 Go 编码 PNG。

Go 使用 github.com/goexlib/cgo 动态加载小型 C 适配库，Go 二进制使用 CGO_ENABLED=0 构建。VNC 采用 libgvnc 核心，RDP 采用 FreeRDP 3 软件解码。C 适配层封装结构体、线程、回调和内存所有权，原生回调不进入 Go。库通过版本化 ABI 提供连接状态、画面快照和输入；不依赖 X11 客户端或显示服务。

## 公共接口

```sh
remote-computer-ctl list --json
remote-computer-ctl status mac --json
remote-computer-ctl connect mac
remote-computer-ctl disconnect mac
remote-computer-ctl connect --all
remote-computer-ctl disconnect --all

remote-computer-use --conn mac screenshot --output screen.png
remote-computer-use --conn mac screenshot --output - > screen.png
remote-computer-use --conn mac move --x 300 --y 200
remote-computer-use --conn mac click --x 300 --y 200 --button left --count 2
remote-computer-use --conn mac scroll --dy 3
remote-computer-use --conn mac drag --from 100,200 --to 500,300 --duration 500ms
remote-computer-use --conn mac key CMD+L
remote-computer-use --conn windows type --text Hello
remote-computer-use --conn windows type --stdin
```

所有桌面操作显式指定连接，不自动连接。截图必须指定输出文件或 `-`，原始尺寸 PNG，不额外合成光标。PNG stdout 不能混入 JSON 或日志。坐标以截图左上角为原点，越界报错；滚轮 dy 正值向下，dx 正值向右，单位为刻度。组合键顺序按下、逆序释放。文本与快捷键分开，使用 RDP Unicode / VNC keysym；显式 clipboard 方法仅在后端报告支持时允许，不静默降级。文本输入内容不记入日志。

操作支持 --json、--timeout；坐标动作支持 --geometry-version，拒绝来自旧尺寸截图的操作。返回成功表示事件已发送，不等于远端应用操作成功，AI 应再次截图确认。首期不公开跨命令持续按住键或鼠标的接口，拖拽与组合键自动释放。

## 配置与安全信息

配置默认位于 ~/.remote-computer-use/config.toml，采用 version = 1 和 [connections.NAME]。字段包括 protocol、host、port、username、password/password_env、domain、RDP width/height、可选证书指纹。连接名是非敏感别名，限制为字母、数字、下划线和连字符。

配置文件权限 0600，运行目录 0700。运行目录使用 XDG_RUNTIME_DIR/remote-computer-use 或 /tmp/remote-computer-use-UID。配置与运行目录支持显式覆盖，便于隔离测试。

PrivateConnectionConfig 与 PublicConnectionInfo 必须分离。公开输出只包括别名、协议、状态、尺寸、尺寸版本、帧序号、支持能力及安全错误码。不输出地址、端口、帐号、密码、令牌、证书材料、配置原文。凭据经私有启动管道传递，不放入子进程 argv。底层日志被拦截，不透传原始库错误，不提供显示秘密的调试开关。

TLS 默认验证服务端证书，可显式配置固定指纹。传统无 TLS VNC 需要显式 allow_unencrypted = true，不自动降级。认证成功不表示桌面已解锁。

## 生命周期

状态为 stopped、connecting、ready、disconnecting、failed。connect 等待认证和首个完整画面，默认 30 秒。重复 connect 健康连接、disconnect 已停止连接是幂等操作。配置变更在重新连接时生效。

connect --all 连接当前配置，最多并发 4 个，每项独立返回结果，部分失败不回滚成功连接。disconnect --all 清理当前用户运行的所有连接，包括已从配置删除的连接。任一失败整体退出非零。

同连接操作串行，单动作默认超时 10 秒，可覆盖。断网停止接收输入，不重放动作。取消、超时、失败时尽力释放本次操作按下的键和按钮。尺寸变化使旧画面无效，等待新完整画面后恢复输入。截图只表示最新接收到的完整画面，不是应用完成屏障。断开不主动注销远端用户。

## 实施阶段与验证

1. 设计文档和仓库约定；独立提交。
2. Go CLI、配置校验、私有 IPC、进程生命周期、脱敏查询、批量操作和测试。
3. 原生 ABI、libgvnc 后台截图和输入；真实回环 VNC 集成测试。
4. FreeRDP 3 后端、共享输入语义、异常和内存所有权验证。
5. 构建打包、使用文档；CLI 稳定后适配 Codex/OpenCode/Claude Code Skill。

验收必须覆盖无 DISPLAY/WAYLAND_DISPLAY、截图颜色与尺寸、增量更新、坐标边界、组合键释放、双击/拖拽/滚轮、Unicode 支持范围、多连接隔离、批量部分失败、错误认证、证书失败、断网、取消、进程崩溃和秘密泄露检查。不能把 mock 测试或 Linux 回环测试当作 macOS/Windows 实机验证；未具备实机时明确记录兼容性待验证。

提交使用 Conventional Commits。每个阶段运行相应测试，保持提交可审阅，不提交配置凭据或构建产物。
