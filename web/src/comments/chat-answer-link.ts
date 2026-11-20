export type ChatAnswerLink = { before: string; label: string; runId: string }

const trailingLink = /^([\s\S]*?)\[([^\]\n]{1,40})\]\(\/c\/([0-9a-f-]{36})\)$/

export function chatAnswerLink(
  body: string,
  runId: string | null
): ChatAnswerLink | null {
  if (runId === null) return null
  const match = trailingLink.exec(body)
  if (match === null || match[3] !== runId) return null
  return { before: match[1], label: match[2], runId }
}
