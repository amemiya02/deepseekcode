# deepseekcode 文档

按你的目的选择入口：

## 我想用 —— 学习路径（guide/）

> 渐进式中文教程，按编号顺序读；1→2 是必经，之后可跳读。

<!-- 各教程任务完成时在此追加条目 -->
- 1. [入门：安装到第一个会话](guide/getting-started.md)
- 2. [日常工作流：一次真实的改 bug 走查](guide/core-workflow.md)

## 我想查 —— 参考手册（reference/）

- [config](reference/config.md) — 配置项全量参考
- [providers](reference/providers.md) — Provider 矩阵（DeepSeek / OpenAI-compat）
- [tools](reference/tools.md) — 14 个内置工具
- [permissions](reference/permissions.md) — 权限模型
- [sandbox](reference/sandbox.md) — OS 沙箱（sandbox-exec / Landlock）
- [skills](reference/skills.md) — SKILL.md 发现与渐进披露
- [hooks](reference/hooks.md) — 五类 hook event
- [mcp](reference/mcp.md) — MCP 服务器接入
- [lsp](reference/lsp.md) — LSP 集成
- [commands](reference/commands.md) — 自定义 slash 命令
- [tui-theme](reference/tui-theme.md) — TUI 主题
- [notifications](reference/notifications.md) — 通知
- [pricing](reference/pricing.md) — 价格速查
- [web](reference/web.md) — web_fetch / web_search 工具
- [desktop](reference/desktop.md) — 桌面端
- [bash-validation](reference/bash-validation.md) — Bash 命令校验
- [prefix-cache](reference/prefix-cache.md) — 前缀缓存（用户视角）
- [duet](reference/duet.md) — Duet 验证器（用户视角）

## 我想改 —— 开发文档（dev/）

<!-- 架构总览与深入篇任务完成时在此追加条目 -->
- [architecture](dev/architecture.md) — 架构总览：三端一核、请求生命周期、37 包地图、代码导读 ⭐ 从这里开始
- [agent-loop](dev/agent-loop.md) — 深入：turn 循环、finish-reason 覆写、compaction、repair、thinking 门控
- [prefix-cache](dev/prefix-cache.md) — 深入：canonical 序列化器、前缀指纹、golden 守卫、缓存红线
- [routing](dev/routing.md) — 深入：Flash→Pro 升级信号、Duet 验证器、成本权衡
- [tools](dev/tools.md) — 深入：tool 接口、执行管道、快照回滚、新增 tool 实操与缓存红线
- [tui](dev/tui.md) — 深入：Bubble Tea 模型、组件树、补全弹层、theme 实现
- [three-surfaces](dev/three-surfaces.md) — 深入：gateway/SSE hub、Web SPA、Desktop、ACP 三端一核
- [model-compatibility](dev/model-compatibility.md) — DeepSeek V4 wire 约束（贡献者必读）
- [parity](dev/parity.md) — parity 测试场景登记（与 TestParityConsistency 四向绑定）
- [adr/](dev/adr/) — 架构决策记录

## 其他

- [bench/](../bench/README.md) — 缓存基准与 h2h 证据（自成体系）
- `internal/` — 内部材料（竞品分析等），不属于公开文档

### 文档维护规约

- guide/、dev/ 用中文；reference/ 保留原语言；README 双语同步
- 单一事实源：同一信息只在一处详述，其余链接
- 只写已实现的：每个命令、每个代码锚点当下可验证
- dev/parity.md 的表格与测试耦合，格式不得改动
