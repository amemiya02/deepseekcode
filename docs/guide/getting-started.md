# 入门：安装到第一个会话

> **目标：** 30 分钟内装好 `dsc` 并完成第一次对话。
>
> **前提：** 会用终端、持有一个 DeepSeek API key（在 [platform.deepseek.com](https://platform.deepseek.com) 申请）。

---

## 1. 安装（任选一个渠道）

`dsc` 是单文件静态二进制，无运行时依赖。

### Homebrew（macOS / Linux）

```sh
brew install amemiya02/deepseekcode/deepseekcode
```

> **注意：** tap 已配置但尚未发布；v0.1.0 正式发布前，请使用下方其他渠道。

### curl | sh（任意 Unix）

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh
```

固定版本：

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | DSC_VERSION=v0.1.0 sh
```

自定义安装目录（默认 `~/.local/bin`）：

```sh
curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | PREFIX=/usr/local sh
```

### Scoop（Windows）

```sh
scoop bucket add deepseekcode https://github.com/amemiya02/deepseekcode-scoop
scoop install deepseekcode
```

### Go install（需要 Go ≥ 1.23）

```sh
go install github.com/amemiya02/deepseekcode/cmd/dsc@latest
```

二进制安装在 `$(go env GOPATH)/bin`。

### GitHub Releases（手动）

在 <https://github.com/amemiya02/deepseekcode/releases> 下载对应平台的压缩包，解压后将 `dsc` 放到 `$PATH` 下。

支持的目标平台：

| 平台 | 架构 |
|------|------|
| macOS | arm64、amd64 |
| Linux | amd64、arm64 |
| Windows | amd64 |

### 源码构建

```sh
git clone https://github.com/amemiya02/deepseekcode
cd deepseekcode
make build    # → ./bin/dsc
make install  # → $GOPATH/bin/dsc
```

### 验证安装

```sh
dsc -version
```

---

## 2. 配置 API key

### 设置环境变量

```sh
export DEEPSEEK_API_KEY=sk-your-key-here
```

建议写入 shell 配置文件（`~/.zshrc` 或 `~/.bashrc`）使其持久生效：

```sh
echo 'export DEEPSEEK_API_KEY=sk-your-key-here' >> ~/.zshrc
source ~/.zshrc
```

### 中国大陆镜像

如果直连 `api.deepseek.com` 不稳定，可通过 `DEEPSEEKCODE_BASE_URL` 指向镜像地址：

```sh
export DEEPSEEKCODE_BASE_URL=https://your-mirror-endpoint.example.com
```

### 确认配置生效

```sh
dsc -version
```

输出类似 `dsc v0.1.0` 即说明二进制可用；key 的有效性在下一步的 `dsc doctor` 中校验。

---

## 3. 第一个会话

### 启动 TUI（推荐）

```sh
dsc
```

启动后进入交互式终端界面（TUI）。在输入框中输入你的第一个问题，例如：

```
解释一下这个仓库的认证流程
```

发送后你会看到 dsc 流式输出回答。如果启用了 thinking 模式，推理过程会以折叠块呈现，展开可查看完整思考链。

**常用快捷键（TUI 内）：**

- `↑` / `↓`：浏览历史输入
- `Ctrl+C`：中止当前响应
- `:q` 或 `Ctrl+D`：退出

### 一次性模式（无需 TUI）

适合脚本或快速查询：

```sh
dsc -p "用一段话概括这个仓库的作用"
```

### 只读模式

仅检查代码、不允许 dsc 执行写操作：

```sh
dsc --read-only
```

---

## 4. 自检

安装完成后运行一次自检，确认环境配置正确：

```sh
dsc doctor
```

输出示例：

```
dsc doctor
----------
  [PASS] key-present: key found (sk-**...**1234)
  [PASS] key-valid: API key accepted by server
  [PASS] base-url-reachable: reached https://api.deepseek.com (HTTP 200)
  [FAIL] proxy-configured: no HTTP(S) proxy env vars set (OK if direct access)
  [PASS] cache-fields-in-probe: cache fields present (hit=0 miss=5)
  [PASS] sandbox-available: seatbelt sandbox available

All checks passed.
```

**各检查项说明：**

| 检查项 | 含义 |
|--------|------|
| `key-present` | 当前 provider 能解析到 API key（检查 `DEEPSEEK_API_KEY` 或配置文件） |
| `key-valid` | 向 API 发送探测请求，确认 key 被服务端接受（返回 HTTP 200） |
| `base-url-reachable` | 对 `api.base_url` 做 HEAD 请求，确认基础网络可达（401 也算可达） |
| `proxy-configured` | 检测 `HTTPS_PROXY`/`HTTP_PROXY` 等代理环境变量是否已设置（信息项，不设也不是错误） |
| `cache-fields-in-probe` | 确认响应的 `usage` 块包含 `prompt_cache_hit_tokens` 和 `prompt_cache_miss_tokens`，cost 统计依赖这两个字段 |
| `sandbox-available` | 检测 OS 原生沙箱是否可用（macOS 为 `sandbox-exec`/seatbelt，Linux 为 Landlock） |

> `proxy-configured` 显示 FAIL 不影响正常使用——只是提示当前没有设置代理环境变量，直连正常则忽略即可。

---

## 5. 升级与卸载

### 升级

检查是否有新版本并打印升级命令（默认不执行）：

```sh
dsc upgrade
```

直接执行升级：

```sh
dsc upgrade --apply
```

仅检查版本，不打印升级命令：

```sh
dsc upgrade --check
```

`dsc upgrade` 通过检测二进制自身路径自动判断安装方式，并给出对应的升级命令（brew / curl | sh / go install / 手动下载）。

### 卸载

```sh
rm "$(command -v dsc)"    # 删除二进制
rm -rf ~/.deepseek        # 删除所有会话、快照、配置
rm -rf .deepseek          # 在每个项目目录下执行，删除项目级指针与快照
```

---

## 下一步

- [配置参考](../reference/config.md) — 所有配置项的完整说明（`~/.deepseek/config.toml`、环境变量、provider 矩阵）
- [核心工作流](core-workflow.md) — 如何在真实项目中高效使用 dsc（待上线）
