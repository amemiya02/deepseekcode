# 省钱之道：前缀缓存与智能路由

> **目标：** 理解成本结构，亲眼看到缓存命中。
>
> **前提：** 已完成[外部集成教程](integrations.md)，或直接对成本话题感兴趣。

---

## 1. 为什么 dsc 便宜

### 缓存命中 vs 未命中的量级差

DeepSeek 对**缓存命中**的输入 token 定价约为**未命中的 1/50**（`deepseek-v4-flash`：0.02 vs 1.0 ¥/1M token）。缓存按请求体的字节前缀匹配：从第一个不同的字节开始，后面全部按未命中计价。

一次多 turn 会话的成本结构大致是：

```
[静态前缀：系统提示 + 工具 schema]  → 字节稳定 → 每 turn 只付 0.02 ¥/1M
[追加式对话 body：历史 user/assistant]  → 只追加不重写 → 旧消息同样按命中计价
[本 turn 新增：用户输入 + 动态上下文]  → 每 turn 唯一 → 按 1.0 ¥/1M 计价
```

只要前缀字节稳定，每一轮新请求只需为末尾新增的几百 token 付全价，其余全部享受 50× 折扣。

### 94.7% / 4.5× 这两个数字从哪来

bench 基准测试显示，在同等任务下 `dsc` 达到 **94.7% 缓存命中率**，带来约 **4.5× 的成本优势**（prefix A/B 对照实验）。数字来源：[dev/prefix-cache.md](../dev/prefix-cache.md)，原始跑分记录在 [bench/README.md](../../bench/README.md)。

这两个数字的前提是"前缀字节稳定"——`dsc` 通过以下机制保证这一点：

- **单一序列化器**：`MarshalCacheStable` 是唯一的 wire 序列化路径，工具按名称排序、JSON Schema 键规范化，消除非确定性；
- **静态前缀冻结**：会话开始后前缀被锁定（`PrefixEpoch`），工具 schema 或系统提示的中途变更记为 `PendingChange`，不会悄悄破坏缓存；
- **漂移检测告警**：`PrefixMonitor` 在运行期检测字节级漂移并触发显式事件，而非静默涨价。

机制细节见 [前缀缓存参考](../reference/prefix-cache.md)。

---

## 2. 亲眼看见命中

### 实操步骤

用 `-trace-jsonl` 旗标运行一次 one-shot 任务，把 trace 写入文件：

```bash
dsc -p "用一句话描述这个项目" -trace-jsonl /tmp/t.jsonl
```

然后用 `trace inspect` 解读 trace：

```bash
dsc trace inspect /tmp/t.jsonl
```

输出示例（格式来自 `internal/traceinspect`，由 `dev/prefix-cache.md §8` 核实）：

```
cache 99.6% | hit 48700000 | miss 190000 | saved CNY 47.73 | prefixes 1 | expected_miss 1
```

### 各字段含义

| 字段 | 含义 |
|------|------|
| `cache %` | 全 trace 的缓存命中率 |
| `hit` / `miss` | 命中/未命中 token 数 |
| `saved CNY` | 相对于全部按未命中计价所节省的金额 |
| `prefixes` | 整个 trace 中出现过的不同 `static_prefix_hash` 数量 |
| `expected_miss` | 预期必须 miss 的次数（每个 epoch 首 turn 必冷启动一次） |

**`prefixes 1` 是健康的判据**：整个 trace 只出现一个静态前缀哈希，说明前缀在会话中全程字节稳定。若看到 `prefixes 2+`，说明前缀在会话中途发生了变化（常见原因：某个工具的 `Description()` 或参数 schema 返回了不确定的值）。

`expected_miss` 通常等于 epoch 创建数；冷启动的第一次 miss 是正常的，不是漂移。

---

## 3. Flash→Pro 路由与 Duet

`dsc` 默认用 `deepseek-v4-flash`（低成本、高速）驱动主循环，只在两类时刻自动升级到 `deepseek-v4-pro`：

1. **危险工具门控**：即将执行写入 `.git/`、`.env*`、`secret_path_patterns` 匹配路径，或匹配破坏性 bash 命令（`rm -rf`、`git push --force`、`kubectl delete` 等）时，Pro 先做一次独立验证；
2. **连续修复失败**：同一工具同一参数连续失败两次后，第三次尝试升级到 Pro。

这种"Flash 主力 + Pro 外科介入"的模式，让绝大多数 turn 享受 Flash 的低价，仅在真正需要 Pro 判断力的时刻付出溢价——两全其美。

Duet 的触发逻辑与配置选项见 [Duet 参考](../reference/duet.md)。

---

## 4. 价格速查

完整的 token 单价表（Flash/Pro 各档次，含缓存命中折扣）见 [价格参考](../reference/pricing.md)。

---

## 下一步

教程系列到此完结。想深入了解实现原理，推荐从架构总览开始：[dev/architecture.md](../dev/architecture.md)。
