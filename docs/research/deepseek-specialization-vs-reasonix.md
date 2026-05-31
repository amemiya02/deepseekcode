# DeepSeek 特化深度对比：`deepseekcode` (dsc) vs `DeepSeek-Reasonix`

*生成日期：2026-05-31 · 来源：双方代码库全量审计 + DeepSeek V4 技术报告 (58 页) · 置信度：高（结论均有 file:line 证据；少数处已标注不确定）*

> 评判口径：本报告只回答**"针对 DeepSeek（尤其 V4）的特化深度"**这一个问题。
> 产品广度（桌面端 / Web 仪表盘 / 远程通道等）单独列出，**不计入"特化"得分**——见 §8。

---

## 1. 执行摘要 / 结论先行

两个产品的**核心命题完全相同**：DeepSeek-native、把"前缀缓存稳定性"当作整个 loop 的不变量来换取 50× 缓存命中折扣。这是一场正面对位。

**结论：在"对 DeepSeek V4 的特化深度"这个维度上，`deepseekcode` 总体领先**，尤其在三个最高杠杆的机制上：

1. **缓存稳定性是"构造级保证"而非"约定"** —— 我们有 canonical 序列化器（`MarshalCacheStable` + 共享的 `canonicalizeTools`），指纹 == DeepSeek 缓存键 *by construction*；Reasonix 靠"append-only + 不要乱动前缀"的约定，**没有 canonical 序列化器、不排序 tools、不规范化 JSON-Schema**，对上游重排序无防御。
2. **多模型路由是"真接线"而非"文档"** —— 我们的 Duet pro 校验器（破坏性调用上触发）与 `<<<NEEDS_PRO>>>` / ≥3 错误自动升级**都已实装**；Reasonix README/架构文档里大书特书的 flash→pro 自动升级 **根本没实现**（只存在于 prompt 文案 + 一条事件正则 + 一个合成测试里）。
3. **自动推理强度选择** —— 我们 `SelectThinking` 按多语言关键词逐回合自动开关 thinking、`reasoning_effort` 有 low/med/high/max；Reasonix 只有手动 `/effort`（默认 high）。这正好命中 V4 报告 p44 "简单任务别 over-thinking"的告诫。

**Reasonix 真正领先我们的，只有两点是"DeepSeek 特化"**：

- **真·V4 BPE 分词器 + 完整 chat template**（`tokenizer.ts`，从 `deepseek-tokenizer.json.gz` 加载）——精确 token 计数。我们是 `bytes÷4 + 每会话 EMA 校准`，长会话能收敛到准、冷启动偏粗。**这是我们唯一一个"真·缺失"的深度特化项。**
- **repair 认识真正的 XML `<｜DSML｜…>` 信封**（V4 原生工具调用格式，p30）。我们的 `ScavengeToolCalls` 只认 JSON 形状泄漏（且把它误注释成 "DSML"），不认 XML 信封。

Reasonix 其余的"领先"（桌面端、Web 仪表盘 + REST API、QQ 远程通道、ACP 编辑器协议、229 测试 + Stryker 变异测试、npm 发行 + Discord 社区）几乎全是**产品广度与成熟度**，**不是 DeepSeek 特化**。在纯特化深度上，我们以更小的代码量（~76K Go vs ~124K TS）做到了更严谨。

---

## 2. 评判基准：V4 技术报告说"好的 DeepSeek 特化"该做什么

把技术报告里与"编码 agent 如何特化"直接相关的事实抽出来，作为打分标尺：

| # | V4 报告依据 | 对 agent 的含义 |
|---|---|---|
| C1 | **On-Disk KV Cache**：跨请求复用按"共享前缀"匹配磁盘上的压缩 KV 块，**复用到最后一个完整压缩块为止**，尾部残块重算 (p23)；Quick Instruction 用特殊 token 直接复用已算 KV、降 TTFT (p32) | 模型可见前缀必须**逐字节稳定**，任何漂移都打断共享前缀匹配、强制重算 |
| C2 | **交错思考持久化**：工具调用场景下推理内容**跨工具轮、跨 user 边界完整保留**；建议用**原生 tool-call 路径**而非"用 user 消息模拟工具"（Terminus 那种拿不到持久化，建议改用 non-think）(p31–32) | 必须走原生 `tool_calls`，并把 `reasoning_content` 正确回传/修复 |
| C3 | **原生工具格式是 XML `\|DSML\|`**：`string="true\|false"` 标注参数类型；XML "缓解转义失败、降低 tool-call 错误" (p30) | 解析/修复应认识 DSML XML 信封 |
| C4 | **训练沙箱 DSec**：原语是 **命令执行 + 文件传输 + TTY**，跑在 Docker 兼容容器/microVM 里 (p35)；自研评测脚手架就是 **bash + file-edit，500 步，512K 上下文** (p37) | 工具面应以 shell + 文件编辑为中心、最好有真 TTY 与真隔离 |
| C5 | **三档推理强度** Non-think/High/Max（Max 往 system 前缀注入特殊指令）(p29)；开发者调研指出 V4 会 **over-thinking 简单任务**、**误读模糊 prompt** (p44) | 应能按任务难度自动调推理强度、对简单任务降档 |
| C6 | **变体取舍**：Pro 1.6T/49B 最强；Flash 284B/13B 更便宜，"给足思考预算时推理可比"，但**知识更弱、高难 agent（尤其 Terminal-Bench）更弱** (p4,p6,p38) | 应有 Pro/Flash 智能路由：默认便宜、难任务升级 |
| C7 | **1M 原生上下文**，但 **128K 后检索下滑**（MRCR：8k≈.94 / 128k≈.90 / 512k≈.76 / 1M≈.66）(p39–40)；V4 KV 体积仅为 V3.2 的 7–10%、为 BF16 GQA8 基线的 ~2% (p5,p13) | 长会话保留全历史成本低（倾向少截断），但要管好 128K 后的有效性；精确 token 计数有价值 |
| C8 | **确定性是一等目标**：batch-invariant 确定性 kernel、训练/推理 bitwise 可复现 (p18,p34) | 可复现/可回放的 agent run 与缓存稳定性测试是契合方向 |

> 报告中**没有**给出"缓存命中价格折扣"的具体数字，也**没有** FIM（fill-in-the-middle）相关内容（全文检索为零）。所以"50× 折扣"是产品侧的真实定价事实，不是报告原文；FIM 两家都没接线，不构成特化差距。

---

## 3. 双方快照

| | **deepseekcode (`dsc`)** | **DeepSeek-Reasonix** (`reasonix`/`dsnix`) |
|---|---|---|
| 语言/形态 | Go 单文件二进制，~76K LOC，213 测试文件 | TypeScript/Node≥22 ESM 单体仓，~124K LOC，229 测试 + Stryker 变异测试 |
| 架构 | 回调驱动 ReAct loop（仿 crush）+ 手写 `internal/llm` HTTP+SSE | ports/adapters 事件日志内核 + `CacheFirstLoop`（~1150 LOC）+ 手写 `fetch`+SSE |
| 持久化 | 纯 Go SQLite（无 CGO），按引用分支 | JSONL append-only + 侧车文件 |
| 发行 | goreleaser（darwin/linux/windows×amd64/arm64）+ Homebrew + Scoop + install.sh | npm `reasonix`/`dsnix`（有下载量）+ Tauri 桌面端 + Web 仪表盘 + Discord |
| 定位 | 终端专属，"by design"（无 GUI） | 终端 + 桌面 GUI + Web 仪表盘 + QQ 远程通道 + ACP |

> 关键交叉验证：两家定价**一致**（我们 CNY：flash 0.02/1.0/2.0、pro 0.025/3.0/6.0 ¥/1M；Reasonix USD：flash 0.0028/0.14/0.28、pro 0.003625/0.435/0.87 $/1M——按 ~7.1 汇率几乎逐项相等）。说明两家都用了真实 V4 定价，定价准确度上**平手**。

---

## 4. V4 特化深度记分卡

评级：✅ 我们更深 ／ 🟰 平手 ／ ⚠️ Reasonix 更深

| 基准 | 我们 (dsc) | Reasonix | 谁更深 | 依据 |
|---|---|---|---|---|
| **C1 前缀缓存逐字节稳定** | canonical 序列化器；指纹==缓存键 by construction；epoch freeze + capability/drift 分层；golden+parity 测试；`dsc trace inspect` 能用 `prefixes==1` **证明**稳定 | append-only + immutable-prefix 约定 + 指纹漂移守卫；**无 canonical 序列化器、不排序 tools、不规范化 schema** | ✅ **我们** | dsc: `request.go:112`,`static_prefix.go:23/65`,`prefix_epoch.go`,`capability_set.go`,ADR-0001 ／ rx: `runtime.ts:81-88`,`client.ts:153-176`（插入序、不排序） |
| **C2 原生 tool-call + reasoning 回传** | 走原生 `tool_calls`；flatten 成 `{content,reasoning_content,tool_calls}` 回传；`SanitizeForDeepSeek` 补缺失推理防 400 | 同左：`reasoning_content` 一等字段 + resume/fold 时 stamp 缺失推理 | 🟰 平手 | dsc: `request.go:69-70`,`agent.go:1035`,`sanitize.go` ／ rx: `types.ts:39-41`,`loop.ts:227-228` |
| **C3 DSML XML 信封修复** | 只认 JSON 形状泄漏（注释误称 "DSML"）；**不认 XML 信封** | 认识真正的 `<｜DSML｜…>` 全角竖线 XML 信封 + 3 种 JSON 形状 | ⚠️ **Reasonix** | dsc: `scavenge.go:150`（实为 JSON）；全仓无 `｜DSML｜` ／ rx: `repair/scavenge.ts` |
| **C4 工具面贴合 DSec（bash+edit+TTY+隔离）** | bash + edit + **`bash_pty`（真 TTY）** + **真 OS 沙箱**（macOS sandbox-exec / Linux Landlock） | run_command/background + edit；**仅路径围栏，无 OS 隔离、无 TTY 优先** | ✅ **我们** | dsc: `sandbox_darwin.go`,`sandbox_linux.go`,`bash_pty` ／ rx: 路径 `pathIsUnder`，grep 无 seatbelt/landlock |
| **C5 自动推理强度** | `SelectThinking` 逐回合多语言关键词自动开关；`reasoning_effort` low/med/high/max（默认 max） | 手动 `/effort`（默认 high）+ 按模型 on/off；**无逐回合自动选择** | ✅ **我们** | dsc: `auto_reasoning.go:30`,`reasoning_effort.go` ／ rx: `slash/handlers/model.ts:35` |
| **C6 Pro/Flash 智能路由** | Duet pro 校验器（破坏性调用触发，JSON 模式、fail-open、主模型已是 pro 时自跳过）**已实装**；`<<<NEEDS_PRO>>>` / ≥3 错误同回合升级**已实装** | 仅手动 `/model`；**自动 flash→pro 升级（含 `<<<NEEDS_PRO>>>`、≥3 错误、"auto" preset）全部未实现**，只在 prompt 文案+正则+合成测试里 | ✅ **我们（差距很大）** | dsc: `hooks/builtin_duet.go`,`agent.go:994-1031` ／ rx: `prompt-fragments.ts:11-25` vs 缺失的 loop 逻辑 |
| **C7 1M 上下文管理 + token 计数** | 1M 感知；双层（确定性+语义）压缩、tool-pair 安全边界、溢出恢复重试；**token：bytes÷4 + 每会话 EMA 校准**（学真实 `usage.PromptTokens`） | 1M 感知；多档阈值压缩（.75/.78/.80/.90）、头部保留 fold；**token：真·BPE 分词器 + V4 chat template** | 🟰 拆分：上下文管理🟰 / **token 计数⚠️ Reasonix** | dsc: `compact.go:342`,`agent.go:567`(EMA) ／ rx: `context-manager.ts:27-41`,`tokenizer.ts` |
| **C8 确定性 / 可复现 / 可证明** | trace JSONL + `dsc trace inspect`（cache% / saved CNY / prefixes）；确定性压缩路径；缓存稳定性 golden + 9 个 parity 场景 | replay/events/JSONL；缓存遥测端到端；但缓存稳定性靠约定，无 canonical golden | ✅ 略偏我们 | dsc: `traceinspect.go:262`,`PARITY.md` ／ rx: `telemetry/`,`transcript/diff.ts` |

**净结果**：C1/C5/C6 我们明显更深，C4/C8 略偏我们，C2 平手，C7 拆分（上下文管理平手、token 计数 Reasonix 深），C3 Reasonix 更深。

---

## 5. 我们的优势（优势）

1. **缓存稳定性是构造级保证，且可证明。** 单一 `canonicalizeTools` 同时喂给 wire 序列化器和指纹，wire 字节与缓存键**不可能发散**（`static_prefix.go:23`）；`TestCacheStableGolden` 钉死字节、`TestFingerprintTracksWireStaticHead` 钉死二者联动。Reasonix 的"byte-stable"是*不去乱动*换来的，对 tool 注册重排或 schema 键序非确定**无防御**（`client.ts:153-176` 插入序、不排序、不规范化）。而且我们能用 `dsc trace inspect` 的 `prefixes==1` **从 trace 证明**一次会话全程缓存稳定——Reasonix 的 "99.82% 命中" 是某用户仪表盘截图的轶事（`benchmarks/real-world-cache/`），非代码可复现指标。

2. **Epoch / Capability / Drift 三层架构（ADR-0001）。** 把"模型可见字节"（进指纹）与"潜在身份"（profile/skills/MCP，只决定构建哪个前缀、不进指纹）干净分离：技能编辑只报 1 个 `skill_body_changed`、MCP 键序重排不产生漂移、profile 改名不挪指纹（不浪费缓存 miss）。`FreezeEpoch` 冻结后强制用快照，运行中 prompt/tool 漂移**打不破缓存**。Reasonix 有指纹漂移守卫，但没有 epoch/序列/漂移分类系统。

3. **多模型路由是真接线（C6）。** 这是对位中最戏剧性的发现：Reasonix 的招牌"flash 默认、难任务自动升 pro"**没实现**。我们两条路径都实装：Duet pro 校验器（仅破坏性调用、JSON 模式、fail-open、主模型已是 pro 时自跳过）+ `<<<NEEDS_PRO>>>`/≥3 错误同回合升级（重新过预算门、丢弃 flash 计费、回退 storm 历史）。

4. **自动推理强度（C5）直接命中报告告诫。** `SelectThinking` 按多语言关键词（`debug`/`调试`/`デバッグ`…强制开，`search`/`搜索`…强制关）逐回合选 thinking，CJK 友好。报告 p44 明说 V4 爱 over-think 简单任务——我们自动降档，Reasonix 要手动。

5. **真 OS 沙箱（C4），贴合 DSec 哲学。** `sandbox-exec`(darwin) / Landlock(linux) 真隔离 + `bash_pty` 真 TTY，正好对应 DSec "命令执行+文件传输+TTY+容器隔离"原语。Reasonix 只有词法路径围栏（grep 无 seatbelt/firejail/landlock），"sandbox root" 只是 cwd 边界。

6. **请求前预算投影（C 经济性）。** `ProjectedTurnCostCNY` 在**发请求前**按会话滚动命中率（向 miss 取保守）给本回合定价、缺省 2048 输出 token，超预算直接 `budget_blocked` 不打 API。Reasonix 的 `budgetUsd` 只是**事后**累计支出软门（到 80% 警告、超了拒下一回合），不是对**即将发生**回合的投影。

7. **工程纪律红利**：纯 Go 无 CGO 单文件交叉编译；`requirements.toml` 管理员下限（超限**启动即拒，不夹紧**）；token 比例 EMA 自校准（DeepSeek 改分词器我们自动适应，Reasonix 得重新 vendor 那个 gz）；两层流超时（首 token 45s vs chunk 20s，适配 reasoner 冷启动）。

---

## 6. 我们的劣势（劣势）

### 6a. 属于"DeepSeek 特化"的真劣势（应优先补）

1. **没有真分词器（唯一的真·深度缺口）。** Reasonix 有完整 V4 BPE + chat template（`tokenizer.ts`，gz 数据 + postinstall 解包），token 计数从第 1 个 token 就精确、无需 API 往返即可预判压缩/裁剪。我们 `bytes÷4 + EMA`：长会话收敛到准，但**冷启动/一次性任务偏粗**，且必须等至少一回合 `usage` 才能校准。影响面：压缩触发点、成本显示、预算投影、proactive shrink 的精度。**这是最值得补的一项。**

2. **repair 不认 XML DSML 信封（C3）。** V4 原生工具格式是 XML `<｜DSML｜invoke …>`（p30，"降低 tool-call 错误"）。模型在异常路径把工具调用当 prose/`<think>` 内吐出来时，可能是 XML 信封；Reasonix 认得，我们只认 JSON 形状（`scavenge.go` 只扫 `{...}`）。范围有限（仅修复兜底路径），但**有报告直接背书**，且补起来便宜。

### 6b. 不属于"特化"、但影响竞争的劣势（属产品策略，非本题核心）

3. **零 GUI / 仪表盘 / 远程通道。** Reasonix 有 Tauri 桌面端、Web 仪表盘 + ~30 REST 端点、QQ 远程通道、ACP 编辑器协议。我们终端专属（`design.md` 明确 "OUT (probably never)"），有 `Bus().Subscribe` 为未来 daemon 留口但未发货。**这是 Reasonix 最大的体感"领先"，但它是广度不是 DeepSeek 特化。**

4. **成熟度/社区信号弱。** Reasonix：npm 下载量、Discord 双语社区、335KB CHANGELOG、Stryker 变异测试、229 测试。我们 213 测试、文档扎实，但无公开发行量/社区/变异测试。

5. **`docs/design.md` 已过时**（停留在 v0.1 `deepseek-chat`/`reasoner`、把 subagent/sandbox 称 "deferred"，其实都已发货）。这不是代码缺口，但**会让读者低估产品**——正是本次审计被预警的陷阱。

---

## 7. 平手项 & 需纠正的认知（myth-busting）

- **off-peak 分时定价"缺失"是伪命题。** Reasonix 审计把"扁平 24/7 定价"列为缺陷，但我们 `docs/pricing.md` 证明：当前硬编码表**就是永久折后价（2.5 折==1/4）**，V4 时代无需时段门。两家都扁平，对 V4 都正确——**平手，不是任何一方的缺陷**。
- **reasoning_content 跨轮持久化（C2）：平手。** 两家都走原生 `tool_calls`、都回传 `reasoning_content`、都修复缺失推理防 400。我们一度怀疑自己只 stub 占位符丢了真推理——已验证：正常回合真 `reasoning_content` 照常回传，占位符只是缺失兜底。
- **定价准确度：平手**（§3 已交叉验证逐项相等）。
- **缓存遥测：平手**（两家都解析 `prompt_cache_hit/miss_tokens`、都端到端展示命中率/节省）。
- **工具修复管线：大体平手**（两家都有 scavenge + JSON 修复 + storm 抑制；差异只在 C3 的 XML 信封）。
- **Reasonix README 几处不可证伪**：`<<<NEEDS_PRO>>>` 自动升级、flash→pro "auto" preset、"3+ 错误自动升级"——**代码里都没有**（甚至 prompt 告诉模型"系统会自动升级"，而架构文档自承"没有失败计数阈值"，自相矛盾）。引用 Reasonix 能力时要看代码不看 README。

---

## 8. 关键区分：广度 ≠ 特化深度

用户问的是"**针对 DeepSeek 的特化**"。把 Reasonix 的"领先项"按这把尺子归类：

| Reasonix 领先项 | 是 DeepSeek 特化吗？ |
|---|---|
| 真 V4 BPE 分词器 + chat template | ✅ 是（我们应补） |
| DSML XML 信封修复 | ✅ 是（我们应补，便宜） |
| Tauri 桌面端 | ❌ 否，产品广度 |
| Web 仪表盘 + REST API | ❌ 否，产品广度 |
| QQ 远程通道 / ACP | ❌ 否，集成广度 |
| Stryker 变异测试 / 229 测试 / Discord / npm 量 | ❌ 否，工程成熟度/社区 |

**即：Reasonix 在"DeepSeek 特化"上真正超过我们的只有 2 项（分词器、DSML 修复），其余全是广度/成熟度。** 而我们在最高杠杆的特化机制（C1 缓存构造级稳定、C6 真路由、C5 自动推理强度、C4 真沙箱）上**反超**一个 124K LOC 的对手——以 76K LOC 做到，特化密度更高。

---

## 9. 建议（按优先级 / ROI）

**P0 —— 补齐唯二的真·特化缺口：**
1. **接入真 V4 分词器（纯 Go BPE）。** 内嵌 DeepSeek 的 `tokenizer.json`（或 vendor 等价 BPE），保持无 CGO/单文件。用它做发请求前的精确 token 计数，喂给压缩触发、预算投影、成本 HUD、proactive shrink。保留 EMA 作为分词器缺失时的回退。→ 直接抹平 C7 唯一缺口。
2. **让 `ScavengeToolCalls` 认识 XML `<｜DSML｜invoke …>` 信封**（含 `string="true|false"` 参数类型规则，p30）。小改动、报告直接背书、补强异常路径鲁棒性。

**P1 —— 把既有优势做成可见的护城河（营销/可证明性）：**
3. 把 `dsc trace inspect` 的 `prefixes==1 / saved CNY` 做成一键"缓存稳定性证明"报告——我们能**证明**，对手只能**截图**。这是对 Reasonix "99.82%" 轶事的正面回击。
4. 把"真路由（Duet + 升级）已实装 vs 对手只在文档"、"真 OS 沙箱 vs 路径围栏"、"自动推理强度"写进 README/对比页。这些是已落地却未充分宣传的差异化。

**P2 —— 战略岔路（产品范围，非本题特化）：**
5. **明确决策**：终端专属是否长期战略？若要回应 Reasonix 的广度，最低成本是基于已有 `Bus().Subscribe` 出一个轻量 daemon/GUI 壳或编辑器协议（类 ACP），而非全套桌面端。建议**显式取舍并写进路线图**，避免被"广度差距"被动牵着走。
6. **更新 `docs/design.md`** 到 V4 现状（或明确标注它是历史 v0.1 计划、转指 MODEL_COMPATIBILITY/prefix-cache/CONTEXT 为现行真相），消除"自我低估"陷阱。

**可不做（已确认非差距）：** off-peak 分时定价（V4 已永久折扣）；FIM（报告无、两家都没接、对 agent 价值低）；多模型校验侧模型（报告趋势反而是把辅助模型*折叠进*主模型，p32）。

---

## 10. 来源与方法

- **DeepSeek V4 技术报告**（58 页，preview）：`https://huggingface.co/deepseek-ai/DeepSeek-V4-Pro/resolve/main/DeepSeek_V4.pdf`，经 pdf-mcp 精读 p4-13/21-23/27/35-44。
- **`deepseekcode` 全量审计**：`internal/llm/{request,static_prefix,prefix_drift,cache_metrics,sanitize,reasoning_effort,auto_reasoning,client,provider_deepseek}.go`、`internal/agent/{agent,prefix_epoch,capability_set,compact,budget_projection}.go`、`internal/hooks/builtin_duet.go`、`internal/{permissions,sandbox,lsp,repair,session}/`、`docs/{prefix-cache,pricing,duet,v4-beta-modes,MODEL_COMPATIBILITY,PARITY}.md`、`CONTEXT.md`、ADR-0001。本人另行核验了 `scavenge.go`、`sanitize.go`、DSML/`reasoning_content` 数据流。
- **`DeepSeek-Reasonix` 全量审计**：`REASONIX.md`、`docs/ARCHITECTURE.md`、`src/{client,loop,memory/runtime,context-manager,tokenizer,repair/scavenge}.ts`、`src/telemetry/`、`prompt-fragments.ts`、测试目录计数。
- **方法**：3 个 opus 子 agent 并行按统一 8 维打分（要求 file:line 证据、对双方均"看代码不看 README"）；关键差异点（DSML、reasoning 持久化）由本人回查代码确认；定价做了 CNY/USD 交叉验证。
- **不确定项**：报告未给缓存折扣具体数字（"50×"取自产品定价事实，非报告）；Reasonix `TURN_END_RESULT_CAP_TOKENS=3000` 具体常量未逐字核到（机制存在）。

---

## 11. 附录：一句话记分

> **特化深度**：dsc 领先（C1/C5/C6 明显领先，C4/C8 略领先，C2 平手，C3 落后，C7 上下文平手/分词器落后）。
> **真·特化缺口**：仅分词器 + DSML-XML 修复两项。
> **Reasonix 的体感领先**：主要是桌面/仪表盘/通道/社区等**广度与成熟度**，非 DeepSeek 特化。
> **一句话**：我们用一半的代码量，做出了更严谨的 DeepSeek 特化；补上分词器和 DSML 修复，特化维度可全面压制；广度差距是另一个（需显式决策的）产品战略问题。
