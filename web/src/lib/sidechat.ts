// Helpers for the dsc-only /review and /btw composer commands. They shape the
// prompt the agent receives; the readOnly flag tells the gateway/agent to refuse
// write tools for this turn (a read-only side-chat). The Composer (Wave 2) calls
// these from its slash menu.

export interface ShapedPrompt {
  text: string
  readOnly: boolean
}

export interface SideChatCommand {
  name: string
  descKey: string
}

export const SIDECHAT_COMMANDS: SideChatCommand[] = [
  { name: '/review', descKey: 'slash.review' },
  { name: '/btw', descKey: 'slash.btw' },
]

const REVIEW_PREAMBLE =
  'Review the current diff for correctness, regressions, and reuse/simplification. ' +
  'Do not edit any files; only report findings. Focus:'

const BTW_PREAMBLE =
  'Side question (read-only). Answer concisely. Do not edit, modify, or change any files. Question:'

// isSideChatCommand reports whether the leading token is a recognized side-chat command.
export function isSideChatCommand(token: string): boolean {
  return SIDECHAT_COMMANDS.some((c) => c.name === token)
}

// buildReviewPrompt frames a diff-critique turn that must not mutate the tree.
export function buildReviewPrompt(focus: string): ShapedPrompt {
  return { text: `${REVIEW_PREAMBLE} ${focus}`.trim(), readOnly: true }
}

// buildBtwPrompt frames a read-only side-chat question.
export function buildBtwPrompt(question: string): ShapedPrompt {
  return { text: `${BTW_PREAMBLE} ${question}`.trim(), readOnly: true }
}
