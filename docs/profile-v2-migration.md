# Profile v2 迁移

v2 把执行设置和 Provider 连接配置移出专家库。它不更改内容目录、专家 ID、命名空间、继承、资产格式、
列表省略或空数组的语义。公开版 0.8.1 起 `init` 默认生成 v2；旧通用 v1.1 库仍可读取。

## 新格式

把 `agent-pack.toml` 的 `schema_version` 显式设为 `v2`。源 Profile 和生成的
manifest Profile 条目都不能出现 `type`、`model`、`providers`、`supported_models`、
`permission`，包括空字符串、空数组和空对象。继承结果同样校验。

保留 `description`、`instructions`、`skills`、`subagents`、`extends`、`namespace`、
`envs`。顶层 `providers` 禁止出现，包括 `{}` 和 `null`；生成器不再输出该字段，
源仓也不能包含 `providers/**/*.toml`。模型目录、端点、凭据与连接参数由执行侧管理。
`envs` 是变量名依赖，
不能写入凭据值。消费端自有扩展不因此成为 Contract 字段。

例如 `profiles/common/reviewer.toml`：

```toml
description = "Review changes"
instructions = ["instructions/common/reviewer/AGENTS.md"]
skills = ["review"]
envs = ["REVIEW_TOKEN"]
```

本地渲染必须在调用时提供目标引擎：

```sh
agent-pack-contract check --repo-root .
agent-pack-contract render --repo-root . --profile reviewer --agent codex --model example-model --permission-mode plan
```

省略模型、权限时使用原生引擎默认。显式不支持的权限模式会报错，例如 Pi 的权限模式。
模型名（包括带 `/` 的原生名称）原样传给执行引擎，不再查询专家库 Provider 目录。
模型与 Provider 的实际可用性由运行环境确认；离线渲染不调用模型服务。
执行设置只进入渲染结果和执行快照，不写回专家源文件。

## 升级顺序

1. 先升级生产端和消费者，使其支持两个 manifest 版本。旧消费者应明确拒绝 v2。
2. 逐个记录旧专家的引擎、默认模型、Provider、白名单和权限差异，把实际需要的执行
   设置接到调用方。确认自定义 Provider 已在目标执行环境可选择，再删除库内 Provider
   源文件；不能只把旧别名继续传入 render。
3. 确认工具凭据仍可按 `envs` 请求，再删除五个 Profile 字段并切换库版本。
   已由 0.7.2 生成的 v2 清单也必须用新版重新生成，删除顶层 `providers`，即使原来为空。
   0.7.2 的 Contract validator 会拒绝省略该字段的新清单，须同步升级校验及渲染工具。不能统一把原权限
   改为 bypass；也不能把省略后的原生默认当成已验证的等价迁移。
4. 构建、校验、在目标引擎投影并验收。既有会话继续使用冻结的执行设置；消费者刷新
   专家内容时也应保留这些设置。

Contract 0.8.0 是收紧 v2 协议的版本边界，公开发行版 0.8.1 包含这些规则。
公开版保留 v1.1 的通用内容和标准 openai-compat Pi 投影；需要其它执行集成的使用方
应由执行环境承接。已有运行会话的执行快照不应随清单更新而改写。
