# Windows RDP 证书指纹

当 Windows 已启用 TLS，但使用自签名证书或证书名称不匹配时，客户端可能返回 `TLS_VERIFY_FAILED`。可在确认实际证书后，设置 `certificate_sha256` 固定指纹。

在目标 Windows 本机、或已有可信管理通道中的管理员 PowerShell 执行以下只读命令。它根据 RDP 监听器引用的 SHA-1 定位证书，再计算本项目所需的 SHA-256；不要直接把 Windows 常见的 SHA-1 Thumbprint 填入配置。

```powershell
$rdpSetting = Get-CimInstance -Namespace root/cimv2/TerminalServices -ClassName Win32_TSGeneralSetting -Filter "TerminalName='RDP-tcp'"
$rdpThumbprint = $rdpSetting.SSLCertificateSHA1Hash
if (-not $rdpThumbprint) { throw 'RDP listener certificate reference is unavailable' }
$rdpCert = Get-ChildItem Cert:\LocalMachine -Recurse |
    Where-Object { $_.Thumbprint -eq $rdpThumbprint } |
    Select-Object -First 1
if (-not $rdpCert) { throw 'RDP listener certificate was not found' }
$rdpSha256 = [System.Security.Cryptography.SHA256]::Create()
try {
    [System.BitConverter]::ToString($rdpSha256.ComputeHash($rdpCert.RawData)).Replace('-', '').ToLowerInvariant()
} finally {
    $rdpSha256.Dispose()
}
```

将输出的 64 位十六进制字符串填入本机 `~/.remote-computer-use/config.toml` 的 `[connections.windows]` 段：

```toml
certificate_sha256 = "替换为上面得到的完整SHA256指纹"
```

保存后重新连接：

```sh
remote-computer-ctl connect windows --json
```

固定指纹替代 CA / 主机名校验，仅接受该证书，仍使用 TLS。证书更换后须重新确认并更新指纹。若采用受信任且名称匹配的证书，则无需固定指纹。

监听器证书引用方式依据 [Microsoft 的 RDP 监听器证书说明](https://learn.microsoft.com/en-us/troubleshoot/windows-server/remote/remote-desktop-listener-certificate-configurations)。上述 PowerShell 步骤尚未在本项目的 Windows 实机上执行。
