# 贡献指南

从克隆仓库到发起 PR 的完整流程：配置环境、理解 make targets、跑通测试体系、遵守 checklist。

## 1. 环境

### Go

以 `go.mod` 为准：**Go 1.26.3**。

```sh
go version   # 确认 >= 1.26.3
```

### Web SPA（可选）

修改 `web/` 下的前端代码或跑 Playwright 时需要：

- Node.js（推荐 LTS）
- 依赖：`cd web && npm install --legacy-peer-deps`

仅改 Go 代码时不需要安装 Node；`make build`（纯 Go 二进制）和 `make test` 均不依赖它。

### Desktop 打包（可选，仅 macOS）

`make desktop` 在 macOS 上使用系统自带工具（`sips`、`iconutil`、`plutil`、`codesign`）打包 `.app`，无需额外安装。桌面端整体架构见 [three-surfaces.md](three-surfaces.md)。

---

## 2. make targets 全解

以下列表以 `Makefile` 实际目标为准，一行一说明。

| target | 说明 |
|---|---|
| `build` | 编译 CLI 二进制 `bin/dsc`（不含 SPA）|
| `build-web` | 先构建 SPA，再以 `-tags withwebapp` 编译，SPA 内嵌进二进制；适合本地跑 web UI |
| `install` | `go install` 纯 Go 二进制到 `$GOPATH/bin` |
| `install-web` | `go install -tags withwebapp`，SPA 内嵌，安装到 PATH |
| `run` | `build` 后立即执行 `bin/dsc` |
| `test` | 跑所有 Go 测试（排除 `desktop/`）|
| `test-race` | 同上，加 `-race` 竞态检测 |
| `cover` | 生成覆盖率报告 `coverage.html` |
| `cover-cache` | 仅对 `llm/repair/routing/agent` 四包生成覆盖率，用于检查缓存相关路径 |
| `lint` | `go vet`（静态分析）|
| `fmt` | `gofmt -s -w .`（格式化）|
| `vet` | 同 `lint`（`go vet` 的独立入口）|
| `tidy` | `go mod tidy` |
| `clean` | 删除 `bin/`、`dist/`、覆盖率文件 |
| `web` | 构建 SPA，输出至 `webapp/dist/` |
| `web-test` | 跑 SPA 的 vitest + Playwright 测试 |
| `desktop` | macOS：先 `web`，再运行 `desktop/package-darwin.sh` 打包 `bin/DeepSeekCode.app` |
| `bench-case-study` | 运行 case-study 基准（需先 `build`）|
| `demo-cache` | 在线跑缓存 A/B demo（需 `DEEPSEEK_API_KEY`）|
| `demo-cache-offline` | 使用已提交 fixture 离线跑缓存 demo |
| `demo-headtohead` | 离线/在线 head-to-head 缓存成本对比 |
| `bench-h2h` | dsc vs reasonix h2h 基准（详见 `bench/README.md`）|
| `ci` | CI 门禁：`web-test` + `test` |

**日常开发最常用：**

```sh
make fmt && make lint && make test   # 提交前必跑
make build                           # 验证能编译
make test-race                       # 怀疑竞态时跑
```

---

## 3. 测试体系导览

### 3.1 parity 四向一致性

文件：`internal/llm/parity_*.go`  
入口测试：`TestParityConsistency`

parity 测试守护「同一场景在四个维度（有无 thinking、有无工具）的序列化输出保持一致」。场景登记在 [parity.md](parity.md)，该表格与测试硬绑定，格式不得随意改动。

```sh
go test ./internal/llm/ -run TestParityConsistency
```

新增或修改场景时先更新 `parity.md`，再修改 `internal/llm/parity_scenarios_test.go`。

### 3.2 golden 指纹守卫

文件：`internal/llm/golden_lock_test.go`、`internal/llm/e2e_cache_stable_test.go`  
代表测试：`TestCacheStableGolden`、`TestCacheStableDeterminism`

golden 测试将序列化器的输出锁定成文件指纹（`internal/llm/golden/marshal_cache_stable.golden`）。任何导致 wire 格式漂移的改动都会让它失败，这正是它的目的——保护前缀缓存命中率。详见 [prefix-cache.md](prefix-cache.md)。

需要合法更新 golden 文件时（例如有意改变序列化格式）：

```sh
UPDATE_GOLDEN=1 go test ./internal/llm/ -run TestCacheStableGolden
# 把更新后的 golden 文件一起提交
```

### 3.3 llmtest 离线 mock 回路

包：`internal/llmtest`

`llmtest` 提供一个无网络、无 API key 的确定性 DeepSeek 服务端（基于 `net/http/httptest`）。它复现完整的真实 wire 路径：SSE 分帧、`reasoning_content` 通道、`tool_call` delta、finish-reason 覆写、两级流超时、尾部 usage 帧。

用于 `internal/agent` 的循环语义测试（`loop_mock_test.go`、`loop_nudge_test.go` 等），无需依赖真实模型：

```sh
go test ./internal/agent/ -run TestLoop
```

注意：`llmtest` 导入了 `internal/llm`，因此 `internal/llm` 的测试不能反向导入它（会形成循环依赖）。

### 3.4 竞态检测（test-race）

```sh
make test-race
```

`executeOne` 被并行 goroutine 调用（`sync.WaitGroup` 扇出），其工具调用计数 `toolCallCount` 使用 `atomic.Int64`（`Add` 返回值保证 80% 告警恰好触发一次）。若 `-race` 报告任何路径，都属于需要先排查的新问题。

### 3.5 Web 测试（vitest + Playwright）

```sh
make web-test
# 等价于：cd web && npm install --legacy-peer-deps && npm test
```

详细说明、fixture 约定、截图比对规则见 [`../../web/TESTING.md`](../../web/TESTING.md)（从 `docs/dev/` 相对路径可达 `web/TESTING.md`）。

---

## 4. PR checklist

提交前逐项核对：

- [ ] **格式与静态检查**：`make fmt && make lint`（`gofmt` + `go vet` 均无报错）
- [ ] **测试全绿**：`make test`；涉及并发改动时加跑 `make test-race`
- [ ] **README 双语同步**：修改了用户可见内容（功能、命令、配置项）时，`README.md`（英文）与 `README.zh-CN.md`（中文）必须同步，使用相同的 `##` 层级结构。此规则仅适用于两份 README，不适用于 `docs/` 下其他文件
- [ ] **文档跟进**：改了用户可见行为（新增命令、修改配置项、调整输出格式）→ 检查 `docs/reference/` 或 `docs/guide/` 中对应页面是否需要更新
- [ ] **新增或修改 tool**：必须跑缓存守卫测试，确保序列化格式不漂移；红线规则见 [tools.md — 缓存红线](tools.md)，缓存机制原理见 [prefix-cache.md](prefix-cache.md)
- [ ] **parity 场景变动**：若改动影响四向一致性，先更新 [parity.md](parity.md) 表格，再改测试，二者保持同步

---

## 5. 文档规约

完整规约见 [../README.md](../README.md)（docs 索引页的「文档维护规约」节）。核心四条：

1. **语言分区**：`guide/` 和 `dev/` 用中文；`reference/` 保留原语言（英文）
2. **单一事实源**：同一信息只在一处详述，其余文件链接过去，不重复
3. **只写已实现的**：每个命令、每个代码锚点当下必须可验证——不写计划中的功能
4. **parity.md 表格与测试耦合**：`dev/parity.md` 的格式由 `TestParityConsistency` 解析，不得随意改动结构
