# Task: mcp-schema-drift

## Prompt

Analyze the MCP (Model Context Protocol) configuration in this project. Look for:

1. Schema definitions and their versions
2. Any inconsistencies between schema and implementation
3. Missing or deprecated fields
4. Type mismatches between config and code

Report your findings. If everything is consistent, say so.

**Important**: Do NOT modify any files. Only provide analysis.

## Expected Behavior

- Agent should read MCP-related configuration files
- Agent should check schema consistency
- Agent should identify any drift between config and code
- Agent should report findings clearly

## Success Criteria

- Exit code 0
- No file modifications
- Accurate schema analysis
