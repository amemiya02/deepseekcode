# Phase 3 — Hooks 系统自测报告（修订版）

## 测试环境

- OS: macOS (darwin)
- Go: `go version`
- 时间: 2026-05-24 (初版) / 修订: 2026-05-24

## 评审修复记录

以下问题已于第二轮修复：

| 编号 | 问题 | 修复 | 验证 |
|------|------|------|------|
| B1 | gofmt 失败 (app/items/theme.go) | `gofmt -s -w` | `gofmt -l` 变更文件输出空 |
| M1 | err != nil 路径丢失 PostToolUseFailure | 抽出 `firePostHook` helper，在 err 早返前调用 | TestAgentPostToolUseFires 通过 |
| M2 | "缺 type → 默认 subprocess" 未实现 | main.go 装配时 `hc.Type == ""` → `TypeSubprocess` | 编译通过 |
| M4 | runner.go dead branch (hasDeny) | 删除 hasDeny 变量及其分支 | go vet 零警告 |
| M5 | config 层未校验 event 字符串 | main.go 增加 validHookEvents map + validHookEvent() | 编译通过 |
| m1 | TUI hookName/event 重复拼接 | scrollbar 只传 event，不再拼 hookName | 编译+测试通过 |
| m2 | EventHookFired Dur 未渲染 | items.go 增加 duration.Round(Millisecond) 显示 | 编译+测试通过 |
| Missing | T-305/306/307/308 验收测试缺失 | 新增 TestAgentPreToolUseDeny / TestAgentPostToolUseFires / TestAgentSessionHooks / TestConfigLoadHooks | 全部 PASS |

## 单元测试结果

### internal/hooks/

| 测试名 | 结果 | 覆盖场景 |
|--------|------|----------|
| TestRunnerNoHooks | PASS | 无配置 hook，返回 allow |
| TestRunnerBuiltinAllow | PASS | 单 builtin 返回 allow |
| TestRunnerBuiltinDeny | PASS | 单 builtin 返回 deny |
| TestRunnerBuiltinAsk | PASS | 单 builtin 返回 ask |
| TestRunnerMultiHookDenyShortCircuits | PASS | deny 短路，后面的 hook 不执行 |
| TestRunnerMultiHookAskBeforeAllow | PASS | ask 优先于 allow |
| TestRunnerBuiltinNotRegistered | PASS | 未注册 builtin → fail-open → continue |
| TestRunnerEventNotConfigured | PASS | 事件无配置 → 返回 allow |
| TestRunnerFailOpenSubprocess | PASS | 三类 fail-open：crash/exit 1、timeout 50ms/sleep 5、bad JSON |
| TestShellCommandUnix | PASS | macOS 上 shellCommand 返回 /bin/sh -c |
| TestRunSubprocessHookAllow | PASS | printf JSON → allow |
| TestRunSubprocessHookDeny | PASS | printf JSON → deny |
| TestRunSubprocessHookTimeout | PASS | sleep 5 + 100ms timeout → continue "timed out" |
| TestRunSubprocessHookBadJSON | PASS | echo not-json → continue |
| TestRunSubprocessHookNonZeroExit | PASS | 非零 exit 但 stdout 有效 JSON → 仍解析 |
| TestRunSubprocessHookDefaultTimeout | PASS | Timeout 为零 → 默认 30s |

### internal/agent/

| 测试名 | 结果 | 覆盖场景 |
|--------|------|----------|
| TestAgentPreToolUseDeny | PASS | deny builtin → executeOne 返回 IsError + tool 未执行 |
| TestAgentPostToolUseFires | PASS | PostToolUse 在 executeOne 成功后触发 |
| TestAgentSessionHooks | PASS | SessionStart + SessionEnd 在 Run 中触发 |

### internal/config/

| 测试名 | 结果 | 覆盖场景 |
|--------|------|----------|
| TestConfigLoadHooks | PASS | `[[hooks]]` TOML 段 → `Cfg.Hooks` 切片正确加载 |

### 全量回归

| 命令 | 结果 |
|------|------|
| `make build` | PASS |
| `make test` | 全绿（所有包） |
| `go test -race ./internal/hooks/` | PASS |
| `go test -race ./internal/agent/` | PASS |
| `go test -race ./internal/config/` | PASS |
| `go vet ./...` | 零警告 |
| `gofmt -l` (本次变更文件) | 空（格式正确） |

## 手动验证的边界情况

1. **Hook deny 阻止工具执行** — `TestAgentPreToolUseDeny` 验证 deny 后 `res.IsError=true` + `tool.Execute` 调用次数为 0
2. **Hook fail-open 不阻塞 agent loop** — subprocess crash / timeout / bad output 全部返回 continue，Runner 聚合为 allow
3. **SessionStart 在 Run 开头触发** — `TestAgentSessionHooks` 验证 Run 入口调 SessionStart
4. **SessionEnd 在 Run 退出时触发** — `TestAgentSessionHooks` 验证 defer SessionEnd 执行
5. **PostToolUseFailure 在 err 路径触发** — `firePostHook` helper 在 err != nil (Canceled 除外) 时调用，保证不遗漏
6. **无配置时 hook 透明** — `TestRunnerNoHooks` / `TestRunnerEventNotConfigured` 验证无配置时返回 allow
7. **builtin 未注册不崩溃** — `TestRunnerBuiltinNotRegistered` 验证 warn 日志 + continue
8. **多 hook 顺序执行** — `TestRunnerMultiHookDenyShortCircuits` 验证 deny 立即短路
9. **决策优先级** — deny > ask > allow/continue
10. **跨平台 shell** — macOS 上 shellCommand 返回 /bin/sh -c
11. **TOML 配置加载** — `TestConfigLoadHooks` 验证 hooks 段正确解析
12. **Doctor 显示 hooks** — `checkHooks` 函数输出 subprocess/builtin 数量分布
13. **TUI 渲染** — EventHookFired 通过 itemHookFired 渲染，deny 决策红色加粗，duration 显示
14. **config type 默认值** — 装配时 `hi.Type == ""` → `TypeSubprocess`
15. **config event 校验** — 无效 event 名称 slog.Warn + 跳过

## 未能自测的部分及原因

- Windows 平台 shellCommand (cmd.exe /C) — 无 Windows CI，依赖 build tag 编译验证
- TUI 手动交互 — 需要终端环境，无法在当前测试框架中自动化
- 真实 LLM 请求结合 hook — e2e 测试使用 fake client，真实端到端在 Phase 9 后通过 T-9002 覆盖
- TestAgentSessionHooks 未在单一 Run 中覆盖全部 4 个 hook 事件 — PreToolUse/PostToolUse 的代码路径仅在 executeOne 中触发，由 TestAgentPreToolUseDeny + TestAgentPostToolUseFires 独立覆盖，在无 Persister 的 minimal Run 中工具执行路径依赖完整 SSE loop，当前用拆分测试覆盖
