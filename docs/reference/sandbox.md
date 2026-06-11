# Sandbox

deepseekcode can wrap shell tools with the operating system's native sandbox.
Sandboxing is disabled by default so existing workflows keep the same behavior.

## 启用

Add a `[sandbox]` section to `.deepseek/config.toml`:

```toml
[sandbox]
enabled = true
allow_read_paths = ["/usr", "/System", "/Library"]
allow_write_paths = []
allow_network = false
```

When enabled, `bash`, `bash_pty`, and `background_bash` are wrapped before the
child process starts. The current working directory is automatically added to
both read and write allowlists so normal project-local commands can still run.

## 添加可写路径

Grant writes explicitly:

```toml
[sandbox]
enabled = true
allow_read_paths = ["/usr", "/System", "/Library"]
allow_write_paths = ["/tmp/dsc-output"]
allow_network = false
```

If the platform sandbox is unavailable or wrapping fails, the command runs
unsandboxed and the tool result starts with a warning line.

## 已知限制

macOS uses `sandbox-exec` with an inline deny-default SBPL profile. Linux uses
Landlock through a hidden `dsc __sandbox_run` child process. Landlock v1 does
not restrict network access, so `allow_network` is currently enforced by macOS
Seatbelt but documented as best-effort on Linux.

Windows and unsupported operating systems use a noop sandbox.
