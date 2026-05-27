# Task: ctx-long-readonly

## Prompt

You are given a large codebase with many files. Your task is to:

1. Read and understand the project structure
2. Identify the main entry points and key modules
3. Explain the architecture and how components interact
4. List all public APIs and their purposes

**Important**: Do NOT modify any files. This is a read-only analysis task.

## Expected Behavior

- Agent should read multiple files to understand the codebase
- Agent should provide a coherent architectural overview
- No files should be modified
- Cache hits should be significant for repeated context

## Success Criteria

- Exit code 0
- No file modifications (diff_invariants check)
- Meaningful analysis output
