# Hooks

deepseekcode supports lifecycle hooks that fire at named events during the agent loop. Hooks can be either:

- **subprocess**: spawn a shell command, pass JSON on stdin, read JSON from stdout
- **builtin**: in-process Go functions (e.g. the Duet Pro validator)

## Events

| Event | When |
|---|---|
| `PreToolUse` | After permission check, before tool execution |
| `PostToolUse` | After successful tool execution |
| `PostToolUseFailure` | After tool execution that returned an error |
| `SessionStart` | When a new agent session is created |
| `SessionEnd` | When the agent loop exits |

## Decision model

Hooks return one of: `allow`, `deny`, `ask`, `continue`.

- **deny** short-circuits immediately (no further hooks run)
- **ask** wins over allow/continue
- **continue** is neutral (treated as if the hook wasn't there)

**Fail-open**: if a hook crashes, times out, or produces invalid output, it is treated as `continue`.

## Configuration

In `.deepseek/config.toml`:

```toml
[[hooks]]
event = "PreToolUse"
type = "subprocess"
command = "jq '{decision: \"allow\"}'"
timeout_seconds = 10

[[hooks]]
event = "PreToolUse"
type = "builtin"
name = "duet"
```

## Examples

### Log every tool call to a file

```toml
[[hooks]]
event = "PreToolUse"
type = "subprocess"
command = '''
  tee -a /tmp/dsc-tools.log | jq -n '{decision: "allow"}'
'''
```

### Block dangerous bash commands with a custom script

```toml
[[hooks]]
event = "PreToolUse"
type = "subprocess"
command = '''
  python3 -c "
import json, sys
i = json.load(sys.stdin)
if i.get('tool_name') == 'bash':
    args = json.loads(i.get('tool_input', '{}'))
    cmd = args.get('command', '')
    if 'rm -rf /' in cmd or 'mkfs' in cmd:
        print(json.dumps({'decision': 'deny', 'reason': 'destructive command blocked'}))
        sys.exit(0)
print(json.dumps({'decision': 'allow'}))
"
'''
timeout_seconds = 5
```

## HookInput JSON schema

```json
{
  "event": "PreToolUse",
  "tool_name": "bash",
  "tool_input": {"command": "ls -la"},
  "cwd": "/home/user/project",
  "session_id": "abc123..."
}
```

## HookOutput JSON schema

```json
{
  "decision": "allow",
  "reason": "looks safe"
}
```
