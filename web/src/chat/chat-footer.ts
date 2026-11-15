type ChatSummary = { total: number; people: number | null }

function count(n: number, singular: string, plural: string): string {
  return `${n.toLocaleString()} ${n === 1 ? singular : plural}`
}

export function chatFooterText(
  summary: ChatSummary | null,
  more: boolean
): string | null {
  if (summary === null || summary.total === 0) return null
  const chats = count(summary.total, "chat", "chats")
  const from =
    summary.people === null
      ? ""
      : ` from ${count(summary.people, "person", "people")}`
  return `${chats}${from}${more ? ". Older chats load as you scroll." : "."}`
}
