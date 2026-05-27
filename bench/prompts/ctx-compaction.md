# Task: ctx-compaction

## Prompt

This is a long conversation simulation. You will be given a large codebase and asked to perform multiple analysis tasks in sequence. The goal is to test context window management and compaction behavior.

1. First, read the entire project structure
2. Then analyze the main module dependencies
3. Next, trace the execution flow from entry point to core logic
4. Finally, identify potential performance bottlenecks

After each step, summarize what you learned. The conversation may become very long - if context compaction is triggered, continue from where you left off.

## Expected Behavior

- Agent should handle long context gracefully
- If auto-compaction triggers, agent should continue working
- Cache efficiency should be maintained across compactions
- Total output should be coherent despite potential context loss

## Success Criteria

- Exit code 0
- No file modifications
- Coherent output across all analysis steps
