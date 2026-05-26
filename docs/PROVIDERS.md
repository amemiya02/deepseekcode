# Providers

deepseekcode defaults to DeepSeek, but the runtime can also talk to
OpenAI-compatible chat-completions endpoints. Provider definitions live in
`.deepseek/config.toml`; the active provider is selected by name.

## Configuration Shape

```toml
[active]
provider = "deepseek"

[providers.deepseek]
type = "deepseek"
base_url = "https://api.deepseek.com"
env_var = "DEEPSEEK_API_KEY"
first_token_timeout_ms = 45000
chunk_stall_timeout_ms = 20000
```

Secrets resolve in this order: `api_key`, then `env_var`, then
`~/.config/deepseekcode/secrets.toml` or `~/.deepseekcode/secrets.toml`.
Secrets files must be mode `0600` on Unix.

## Examples

### DeepSeek (default)

```toml
[active]
provider = "deepseek"

[providers.deepseek]
type = "deepseek"
base_url = "https://api.deepseek.com"
env_var = "DEEPSEEK_API_KEY"
first_token_timeout_ms = 45000
chunk_stall_timeout_ms = 20000
```

DeepSeek enables thinking, prefix-cache accounting, and JSON mode.

### OpenAI

```toml
[active]
provider = "openai"

[providers.openai]
type = "openai-compat"
base_url = "https://api.openai.com"
env_var = "OPENAI_API_KEY"
default_model = "gpt-4o"
```

OpenAI-compatible providers drop DeepSeek's `thinking` field before sending
requests.

### Self-hosted vLLM

```toml
[active]
provider = "vllm"

[providers.vllm]
type = "openai-compat"
base_url = "http://127.0.0.1:8000"
api_key = "local-dev-token"
default_model = "Qwen/Qwen2.5-Coder-32B-Instruct"
```

For local endpoints, `api_key` can be a placeholder if the server only checks
that a Bearer token exists.

## Capabilities

- `thinking`: provider accepts DeepSeek V4 thinking options.
- `prefix_cache`: provider reports DeepSeek prompt-cache hit/miss metrics.
- `json_mode`: provider supports `response_format = json_object`.
- `max_ctx`: approximate maximum context window used for diagnostics.
