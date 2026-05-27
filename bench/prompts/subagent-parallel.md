# Task: subagent-parallel

## Prompt

You need to perform multiple independent analysis tasks on this codebase. Use subagents or parallel processing if available to speed up the work.

Tasks to complete:
1. List all Go source files and count lines of code
2. Identify all exported functions and their signatures
3. Find all TODO/FIXME comments in the codebase
4. Analyze test coverage patterns

Complete all tasks and provide a consolidated report. Wait for each subagent
to finish and fold its result into the report before you finish — do not leave
a subagent running in the background unattended.

## Expected Behavior

- Agent should attempt parallel execution if supported
- All four tasks should be completed
- Output should be well-organized
- Total execution time should benefit from parallelism

## Success Criteria

- Exit code 0
- All four analysis tasks completed
- Output is coherent and well-structured
