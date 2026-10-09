# Agent Pack Contract

[English](README.md)

Agent Pack Contract 编译、校验可移植的专家内容，并为 Codex、Claude Code、Pi
生成本地工作区。新建专家库使用 **manifest v2**：库只声明指令、技能、子专家、说明、
继承和环境变量名；执行引擎、模型及权限由调用方提供。

## 安装和使用

从 [GitHub Releases](https://github.com/amuxos/agent-pack-contract-releases/releases)
下载组合包，解压后运行 `install.sh --bin-dir "$HOME/.local/bin"`。安装器校验平台归档
SHA-256；公开下载无需账号或私有网络。支持 macOS/Linux 的 amd64/arm64。
组合包沿用 `agent-pack-contract-scm-latest.tar.gz` 名称以兼容现有消费者。

```sh
agent-pack-contract init example-pack
agent-pack-contract check --repo-root example-pack
agent-pack-contract validate example-pack/dist/agent-pack.manifest.json
```

`init` 生成 v2 空库、GitHub workflow 和固定当前工具版本的 `ci/check.sh`；不会覆盖
已有受管文件。添加内容型 Profile 后，使用
`agent-pack-contract render --repo-root example-pack --profile reviewer --agent codex`
进行投影。完整建库示例见 [English README](README.md)。

v2 必须指定 `render --agent`，可选 `--model`、`--permission-mode default|plan|bypass`。
这些参数不写回专家内容；不支持的引擎或权限组合在修改输出前报错。v2 禁止 Profile
执行字段（包括空值）、顶层 Provider 字段及 Provider 源文件。旧通用 v1.1 库仍可读取，
迁移需显式接续执行设置；旧 Pi 投影保留标准 `openai-compat` 连接支持。

通用资产命令为 `import-asset`、`new-custom-asset`。源码与二进制均公开，无需账号即可下载。
开发门禁 `ci/check.sh`，发布门禁 `ci/check-release.sh`；CI 工具链为 Go 1.24.12。

## 许可证

Agent Pack Contract 使用 [MIT 许可证](LICENSE)。第三方组件保留各自许可证，详见
[第三方许可声明](THIRD_PARTY_NOTICES)。每个平台发行包均包含这两份文件。
用户创建的 Pack 内容仍适用其作者选择的许可证。
