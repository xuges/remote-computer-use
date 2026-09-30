# 发布指南

首版版本号为 `0.1.0`，Git tag 为 `v0.1.0`。开发构建仍默认使用 `dev`；不要直接重命名开发包冒充版本包，两个 CLI 的 `--version` 必须与包名一致。

## 构建和验证

在相应平台准备好两个无显示依赖的原生适配库后执行：

```sh
make test
make release VERSION=0.1.0
make package-test VERSION=0.1.0
```

`dist/` 应提供四个平台的二进制归档、一份对应源码归档及各自的 SHA-256 文件。源码、配置示例、Skill、发布说明与实际二进制须对应同一个发布提交；不打包真实配置、端点令牌或测试截图。原生依赖清单与第三方许可资料随包提供。项目使用 MIT，第三方许可保持不变，见 [许可与源码说明](../THIRD_PARTY.md)。

Windows 打包器会先构建禁用图像加载器的 gdk-pixbuf，需要 Meson；不使用系统 gdk-pixbuf 引入的 TIFF/JBIG 链。Go 许可资料由 `scripts/go-notices.py` 从当前模块版本与 Go 工具链生成。不得跳过未知依赖的许可复核。

下载并校验精确原生源码（需 Python、zstd 与网络）：

```sh
python3 scripts/collect-release-sources.py --output build/release-sources-0.1.0 \
  dist/remote-computer-use-0.1.0-linux-amd64.tar.gz \
  dist/remote-computer-use-0.1.0-linux-arm64.tar.gz \
  dist/remote-computer-use-0.1.0-darwin-arm64.tar.gz \
  dist/remote-computer-use-0.1.0-windows-amd64.zip
```

检查生成的 `index.json` 没有错误并覆盖四个包。提交发布源码后执行以下命令；脚本要求干净工作区，验证源文件哈希，只打包索引中的原生源文件，并加入该提交的项目源码、构建脚本、Go 模块源码及 commit 记录：

```sh
python3 scripts/package-sources.py build/release-sources-0.1.0 --version 0.1.0
```

源码下载失败、版本不匹配或缺少补丁时，不能将源码包标为完整。

GitHub Actions 的 `Build self-contained packages` 工作流可手动选择发布提交所在分支/tag，并填写 `0.1.0`。它构建并测试四个平台，上传 Actions artifacts；不自动创建 tag 或发布 Release。仅在全部所需任务成功后收集产物，检查包名、哈希和 CLI 版本。当前本地验证结果见 [验证记录](validation.md)，不能替代该次 CI 记录。

Windows 如触发已记录的 ASR 拦截，应保留失败结果与事件证据，在发布说明中明确安装限制，不跳过测试后标为通过。macOS 的 Developer ID 签名与公证尚未完成；浏览器下载安装路径单独验收。

## 创建发布草稿

审核工作区和发布提交后，创建 `v0.1.0` tag 并推送。准备阶段不自动推送、创建远端 tag 或公开发布。

准备好五个归档与五个校验文件后，可用 GitHub CLI 创建草稿；先核对下列通配符只匹配本次版本的十个文件：

```sh
gh release create v0.1.0 --verify-tag --draft \
  --title 'remote-computer-use v0.1.0' \
  --notes-file docs/releases/v0.1.0.md \
  dist/remote-computer-use-0.1.0-*
```

在草稿中核对资产、源码覆盖、最低系统版本、macOS 未公证与 Windows Defender 风险提示，再决定公开发布。源码包需与二进制一同提供，不能仅用可变的上游下载链接替代。
