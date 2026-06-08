# DeepSeek 定价说明

## 定价表（dsc 内置，永久价）

以下为 `internal/llm/cache_metrics.go` 中硬编码的定价表（¥ / 1M tokens）：

| 模型 | 输入(缓存命中) | 输入(缓存未命中) | 输出 |
|---|---|---|---|
| `deepseek-v4-flash` | 0.02 | 1.0 | 2.0 |
| `deepseek-v4-pro` | 0.025 | 3.0 | 6.0 |
| `deepseek-chat` | 0.02 | 1.0 | 2.0 |
| `deepseek-reasoner` | 0.02 | 1.0 | 2.0 |

`deepseek-chat` 和 `deepseek-reasoner` 是 `deepseek-v4-flash` 的别名（非思考/思考模式），价格相同。

## 官方来源

- URL: https://api-docs.deepseek.com/zh-cn/quick_start/pricing/
- 核对日期: 2026-05-25

## "2.5折 == 原价1/4 == 永久价"等价推导

官方页原文（2026-05-25 核对）：

> v4-pro 于北京时间 2026/05/31 23:59 结束 **2.5折**优惠后，正式调整为**原定价的 1/4**。

- `2.5折 = 0.25×`
- `原价的 1/4 = 0.25×`
- 两者数值相同 → v4-pro 实付价（¥3/¥6）在 5/31 前后**不变**。

因此：**不需要日期闸**。dsc 的定价表已等于"永久价"，2026-05-31 不是 cost 计算的事件节点。

## 为什么 dsc 不需要日期闸

dsc 直接硬编码定价表（不依赖外部 pricing API），且当前数值已是 2026-05-31 之后的最终价。
未来若 DeepSeek 再次调价，手动更新 `internal/llm/cache_metrics.go` 中的 `Prices` map 即可。
`TestPricesGolden` 测试会钉死当前数值，防止意外修改。

## dsc doctor 新鲜度检查

`dsc doctor` 会检查定价表的核对日期（`PricingCheckedDate`）：

- 若距今 **≤ 30 天**：显示 `✓ pricing`，附带核对日期、天数和官方来源 URL。
- 若距今 **> 30 天**：显示 `⚠ pricing`（warn 级别），提示定价表可能过期，建议用户前往官方页面核实。

该检查完全本地执行，不发起任何网络请求。常量定义在 `internal/llm/pricing_metadata.go`。

## 未知模型与 NIM 模型

- 未在 `Prices` 表中的模型，`Cost()` 返回 0。
- HUD（CLI 和 TUI）对未知模型显示 `¥?` 而非 `¥0.0000`。
- NVIDIA-NIM 风格模型（如 `deepseek-ai/deepseek-v4-pro`）使用精确 map 查找，前缀不匹配天然 miss → 显示 `¥?`。

## Cache block unit

DeepSeek V4 的 prompt cache 以 128-token 块为单位命中，由经验测量确认（非推测）。

**测量日期**: 2026-06-07
**测量方法**: `go run ./bench/cmd/cacheprobe -model deepseek-v4-flash -min 900 -max 1300 -step 16`
**二次确认**: `go run ./bench/cmd/cacheprobe -model deepseek-v4-flash -min 1800 -max 2100 -step 16`

**结果**: `prompt_cache_hit_tokens` 在 0→128→256 之间阶梯跳变，每次增量恰好 128，证明缓存块粒度为 **128 tokens**。

这与理论预测 `lcm(4, 128) = 128` 一致。该值馈入 `internal/cacheunit.AlignPadding` 用于前缀对齐。
