# 本机 HTTP 控制接口

CLI 与每个后台连接进程通过 HTTP/1.1 通信。公开的 `remote-computer-ctl`、`remote-computer-use` 命令保持不变，VNC/RDP 会话继续由 worker 持有。

## 当前实现

- 每个 worker 只监听 `127.0.0.1:0`，由系统分配空闲端口；不监听 LAN 或公网。
- 每次启动生成 256 位随机令牌。端口与令牌写入私有运行目录的 `<name>.endpoint.json`；Unix 使用当前 UID 和 0600/0700，Windows 使用当前 SID 的受保护 DACL，允许必要的 SYSTEM/Administrators。
- `POST /v1/rpc`，`Content-Type: application/json`，认证头为 `Authorization: Bearer <token>`。请求/响应使用现有 `core.Request` / `core.Result`；操作为 `status`、`disconnect`、`action`，截图 PNG 在 JSON 中编码为 base64。接口尚是内部版本，不承诺独立客户端兼容性。
- 服务器验证精确的 loopback Host，拒绝浏览器 Origin，不开放 CORS。客户端禁用代理及重定向，避免环境变量把本机令牌或截图转发到代理。
- 响应不缓存；限制报文大小、头部与请求体读取时间。请求取消会传递到动作控制器并释放已按下的键鼠。
- 查询输出不含连接地址、用户名、密码、端口或令牌。原生库日志仍被隔离。
- 同一连接的动作串行、不同连接并行；断开仍等待 worker 生命周期锁释放。文件锁只负责生命周期互斥，与 HTTP 传输分开。

运行目录不再受 Unix socket 路径长度限制。Linux 默认 `$XDG_RUNTIME_DIR/remote-computer-use` 或 `/tmp/remote-computer-use-UID`；macOS 使用同样的短路径默认值；Windows 使用 `%LOCALAPPDATA%/remote-computer-use/run`。所有 CLI 必须使用相同的 `REMOTE_COMPUTER_RUN_DIR`（若设置）。

## 为未来转发预留的边界

HTTP 将操作请求与本机 IPC 细节解耦，后续可以增加“操作节点 / 网关”，把标准请求转给实际持有 VNC/RDP 会话的机器。当前未增加远端 API 地址参数，也未实现网关。

建议优先通过已认证隧道接入节点 loopback 接口；跨网络的正式网关应单独负责 TLS、节点身份、每连接授权、令牌交换与并发限制，不能直接把当前监听地址改成 `0.0.0.0`。转发层须保留超时、取消和错误码语义，并为内部 Host 校验提供正确的目标 Host。机器连接凭据仍保留在实际执行节点，不随操作请求传递。

## 从旧版本升级

旧 worker 使用 Unix socket，与新 HTTP 客户端不兼容。升级前用旧版 ctl 断开本项目连接，换用新版两个 CLI 后再连接。不要仅替换客户端而遗留旧 worker，也不要手工删除仍被持有的锁文件。
