# Skills — Cross-Tool Discovery

`dsc` discovers skill files (`SKILL.md`) from multiple ecosystem directories at session start. Skills are listed in the system prompt's static prefix so the model can reference them by name and open the full instructions with `read_file` when needed.

## Scan Directories

Skills are scanned in priority order (project-level before home-level, dsc-native before cross-tool):

**Project-level** (`$CWD/<dir>`):
1. `.deepseek/skills`
2. `skills`
3. `.opencode/skills`
4. `.claude/skills`
5. `.agents/skills`

**Home-level** (`$HOME/<dir>`):
1. `.deepseek/skills`
2. `.claude/skills`
3. `.opencode/skills`
4. `.agents/skills`

Same-name skills are deduplicated: the first occurrence wins (project-level beats home-level; `.deepseek` beats `.claude`).

## SKILL.md Format

A `SKILL.md` file can use optional frontmatter:

```markdown
---
name: my-skill
description: Short description of what this skill does
---
# My Skill

Full instructions here...
```

- `name` is required in frontmatter.
- `description` is optional.
- Without frontmatter, the first `# heading` is used as the name.

## Progressive Disclosure

Only the skill name, description, and file path are injected into the system prompt. The full skill body is not loaded into the prompt — the model opens it with `read_file` on demand. This keeps the prompt compact and cache-friendly.

## Skills as Slash Commands

Discovered skills are automatically promoted to slash commands. Typing `/skill-name` in the TUI submits the skill's body as the prompt. User-defined commands (from `.deepseek/command/`) take priority over skills with the same name.

## Cache Stability

The `## Skills` section is placed in the **static prefix** of the system prompt (before the dynamic context boundary). This means:

- Skills are loaded **once** at session start — they are not re-scanned mid-session.
- The prefix stays byte-stable across turns, preserving DeepSeek's prompt cache and the 50× cache-hit discount.
- Modifying skill directories during a session will not take effect until the next session. Phase 19 (prefix-drift detection) will warn if the prefix changes unexpectedly.
