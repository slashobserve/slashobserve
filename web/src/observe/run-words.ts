export const privateChatText = "Asked in a private chat"

export function runWords(run: {
  prompt: string
  redacted: boolean
}): string | null {
  return run.redacted ? null : run.prompt
}
