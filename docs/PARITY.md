# Parity 场景登记

本文件登记 `internal/llm` parity 黄金 harness（A4 mock parity）覆盖的全部场景，由 `TestParityConsistency` 强制与 `ParityScenarios()`、`testdata/parity/manifest.json` 以及 `testdata/parity/*.golden.json` 文件集合保持一致。新增、删除或重命名任何一个场景必须同时更新这四处。

重新生成黄金 + manifest 的命令：

```sh
UPDATE_GOLDEN=1 go test -run TestParityGolden ./internal/llm/
```

| 场景 | 说明 |
| --- | --- |
| representative | 代表性请求（系统提示 + 2 工具 + thinking + 工具往返），与 `buildRepresentativeRequest` 同源 |
| plain_text | 仅一个 user 文本回合，无工具、无 thinking，钉住最简 wire 形态 |
| tool_sort | 三个故意乱序声明的工具（zebra/alpha/mango），钉住按 function name 字典序排序 |
| schema_canonical | 工具 schema 含乱序键且嵌套子对象，钉住 schema 的递归键排序 |
| thinking_roundtrip | assistant 含 `ThinkingBlock`，钉住 `reasoning_content` 往返与 thinking 结构体 |
| tool_roundtrip | assistant 的 `ToolUseBlock` + tool 角色的 `ToolResultBlock`，钉住 `tool_calls` 与 `role:"tool"` |

## 有意为之的浅覆盖（Intentionally shallow）

parity harness 的目标**只有一个**：钉住 DeepSeek wire 形态的**字节级确定性**——
即缓存键的稳定性。它是刻意保持浅的，以下内容**有意不覆盖**，以免把一个"缓存稳定性
回归探针"误用成通用集成测试：

- **不验证语义或答案质量。** 只断言装配出的请求/响应字节，不判断模型是否给出了
  "正确"的回答。语义行为由 `internal/llmtest` 的离线 mock 回路（T6.3）覆盖。
- **不覆盖网络/流式时序。** 重试、首 token 超时、断块超时、SSE 逐块到达顺序等由
  `internal/llm/client.go` 的两级超时与其专属测试负责，parity 只看装配后的最终形态。
- **不枚举工具/schema 的全部组合。** 仅取代表性场景钉住"按 function name 排序"
  与"schema 递归键排序"两条不变量；新增工具不需要新增 parity 场景，除非它引入了
  新的排序/规范化路径。
- **不做近似匹配。** 任何字节差异都视为漂移并失败——这正是缓存键的语义，而非测试的
  缺陷。需要"差异容忍"的断言应放到对应的功能测试里，不要弱化这里的精确性。
- **不替代 golden 指纹探针。** `TestCacheStableGolden` 与
  `TestFingerprintTracksWireStaticHead` 才是缓存指纹的权威守卫；parity 是它们的
  场景化补充，而不是替代。

需要更深的覆盖时，请在对应层（`internal/llm` 功能测试、`internal/llmtest` mock 回路、
或 `bench/` 黄金 trace）扩展，而不是往 parity 表里堆场景。注意：本节为散文/列表，
**不得**使用竖线表格——`TestParityConsistency` 会把任何 `|` 开头的行的首列当作场景名。
