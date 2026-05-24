package prompt

// baseSection1Identity is the agent-identity / posture section. The
// current placeholder is replaced by T-102 with the full ~30-line
// version. Kept as a real (if minimal) string so a fresh build
// without T-102 still produces a valid prompt.
const baseSection1Identity = "You are deepseekcode, a terminal coding agent powered by DeepSeek.\n"

// baseSection2Tools is the tool-catalog hint section. Filled by T-103.
const baseSection2Tools = "## Tools\nUse the registered tools to read, edit, and run shell commands.\n"

// baseSection3Style is the output-style section. Filled by T-104.
const baseSection3Style = "## Style\nBe concise. Don't restate diffs.\n"

// BasePromptV1 is the cache-stable static base prompt. The "V1"
// suffix is load-bearing: any change to this constant is a release-time
// bump that intentionally invalidates the DeepSeek prompt cache, so
// downstream tasks (T-102~T-104) own the final content but the name
// stays. Compose order matches the section comments above.
const BasePromptV1 = baseSection1Identity + "\n" + baseSection2Tools + "\n" + baseSection3Style
