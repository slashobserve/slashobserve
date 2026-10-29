import type { SavedPrompt } from "./queries"

export function scopeOf(
  prompt: SavedPrompt,
  principalId: string
): { shared: boolean; text: string } {
  if (prompt.owner.id !== principalId) {
    return { shared: true, text: `By ${prompt.owner.name}` }
  }
  return prompt.shared
    ? { shared: true, text: "Everyone" }
    : { shared: false, text: "Only me" }
}
