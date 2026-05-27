# Task: skill-body-change

## Prompt

You need to update a SKILL.md file to reflect a new feature. The skill currently describes a CLI tool, and we need to add documentation for a new subcommand.

Read the existing SKILL.md file, understand its structure, and add documentation for the new `analyze` subcommand that:
- Takes a file path as input
- Outputs a JSON analysis report
- Supports `--verbose` flag for detailed output

Make sure the new documentation follows the existing style and format.

## Expected Behavior

- Agent should read the existing SKILL.md
- Agent should understand the documentation structure
- Agent should add new content that matches the style
- Changes should be minimal and focused

## Success Criteria

- Exit code 0
- SKILL.md is modified with new subcommand documentation
- Changes are consistent with existing style
