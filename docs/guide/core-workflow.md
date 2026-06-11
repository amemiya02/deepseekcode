# 日常工作流：一次真实的改 bug 走查

> **目标：** 用一个完整场景学会提问、审查、回滚、续接、权限选择。
>
> **前提：** 已完成[入门教程](getting-started.md)，`dsc` 已安装并配置好 API key。

---

## 0. 场景设定

假设你正在维护一个 Go 服务，`POST /orders` 接口对空请求体返回 500 而不是 400。你已经能稳定复现：

```sh
curl -s -X POST http://localhost:8080/orders -H "Content-Type: application/json" -d '{}'
# 期望: 400 Bad Request {"error":"body required"}
# 实际: 500 Internal Server Error
```

在仓库根目录启动 dsc：

```sh
cd ~/projects/myservice
dsc
```

---

## 1. 提问与观察

### 怎么描述任务效果最好

模糊指令会让 agent 花一整轮猜测你的意图。描述 bug 时给出三件事：

1. **现象**（实际行为是什么）
2. **复现路径**（最小复现步骤或命令）
3. **期望**（正确行为应该是什么）

**低效写法：**
```
修一下订单接口的问题
```

**高效写法：**
```
POST /orders 收到空 JSON body `{}` 时返回 500，应该返回 400 并附带 {"error":"body required"}。
入口在 internal/handler/orders.go，复现命令：curl -X POST .../orders -d '{}'
```

具体文件路径或函数名能帮 agent 跳过探索阶段，直接定位到目标代码。

### 流式输出里看到什么

发送后，TUI 输出栏会依次出现：

- **Thinking 折叠块**（若模型开启了推理）：显示"Thought for Ns"，展开可查看完整思考链；不展开也不影响最终结果。
- **工具调用行**：形如 `→ read_file internal/handler/orders.go`，表示 agent 正在读取文件。每次工具调用完成后会继续输出下一步。
- **最终回答**：改动说明 + 修改摘要。

`Ctrl+C` 可以随时中止当前响应，已写入磁盘的文件不会回滚（用 `/undo` 回滚，见第 3 节）。

---

## 2. 审查改动

### TUI 内查看

agent 改完后，输出栏会列出每个被修改的文件。你可以直接在终端里继续追问：

```
刚才改了哪些文件？每个文件做了什么？
```

### git diff 双保险

TUI 不能替代你自己的眼睛。改动落盘后，在另一个终端窗口运行：

```sh
git diff
```

逐行确认逻辑是否符合预期。如果仓库里有测试，此时跑一遍是个好习惯：

```sh
go test ./internal/handler/... -run TestOrders
```

### 在 TUI 内打开 `/models`

若你想在审查过程中换一个模型做二次确认，输入：

```
/models
```

弹出可用模型列表，选中即切换，后续的对话轮次使用新模型。

---

## 3. 不满意就回滚

### `/undo` 的语义

每次 agent 调用文件写入类工具（`write_file`、`edit_file`、`apply_patch`）之前，dsc 会把所有**即将被修改的文件**打快照，存放在：

```
.deepseek/snapshots/<sessionID>/<stepIdx>/
```

`/undo` 恢复最近一步的所有快照，实现**多文件 patch 原子回滚**——即使一次改动涉及三个文件，`/undo` 也一次性把三个文件都还原。

```
/undo        # 回滚最近一步
/undo 3      # 回滚最近三步
```

> **注意：** Bash 命令的副作用（如 `go generate` 写出的文件、数据库迁移）不在快照范围内，`/undo` 无法还原。

### 典型用法

agent 的修复方向不对，直接：

```
/undo
```

然后用更精确的描述重新提问，或者指定另一个实现思路：

```
不要修改 handler，改为在中间件层统一拦截空 body，文件是 internal/middleware/body.go
```

---

## 4. 会话续接

调试途中需要休息，或者 terminal 意外关闭？dsc 会自动持久化每一轮对话，下次可以无缝续接。

### 三种方式

**① 继续本目录的上一个会话（最常用）**

```sh
dsc -c
```

自动找到当前工作目录最近一次会话并恢复，历史消息会重新加载到上下文。

**② 按 ID 恢复指定会话**

```sh
dsc -r <session-id>
```

session-id 在 TUI 状态栏或 `/sessions` 面板里可以复制到。也可以用快捷别名：

```sh
dsc -r last      # 等同于 dsc -c，恢复最近一次会话（跨目录）
dsc -r latest    # 同上
```

**③ 在 TUI 内浏览历史会话**

不用退出当前会话，输入：

```
/sessions
```

弹出会话列表（按时间倒序），选中一条即切换到该会话的上下文。适合在多个并行任务之间跳转。

---

## 5. 权限模式怎么选

dsc 有四个全局模式 flag，在启动时指定，优先级高于所有规则引擎配置：

| Flag | 效果 | 适用场景 |
|------|------|----------|
| `--read-only` | 只读工具自动允许，写入 / 执行类工具全部拒绝 | 只想让 agent 分析代码、不允许它修改任何文件 |
| （默认，无 flag）| 交互式确认——每次写入或执行都弹出提示，你逐个批准 | 日常开发，保持对每一步的控制 |
| `--yolo` | 全部自动允许，无需确认 | 可信沙箱内的批量自动化任务（见 [sandbox](../reference/sandbox.md)） |
| `--ask-all` | 每个工具调用都弹出确认，包括只读工具 | 学习阶段或高度敏感代码库，想看清 agent 的每一步 |

**决策建议：**

- 日常改 bug → 默认模式，逐步确认写入。
- 做代码审阅、写文档、回答问题 → `--read-only`，杜绝意外写入。
- CI 流水线或受控沙箱内的自动化 → `--yolo`，配合 [sandbox](../reference/sandbox.md) 限制写路径和网络。
- 第一次用 dsc 接触新代码库 → `--ask-all`，看清 agent 的行为再放权。

在 TUI 内随时查看当前生效的权限策略：

```
/permissions
```

弹出只读摘要，显示当前模式、bash allowlist 大小、规则引擎是否激活。完整权限文档见 [permissions 参考](../reference/permissions.md)。

---

## 下一步

- [配置：从默认值到项目级定制](configuration.md) — 四层叠加模型、模型与 effort 选择、界面语言与主题切换
