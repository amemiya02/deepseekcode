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
