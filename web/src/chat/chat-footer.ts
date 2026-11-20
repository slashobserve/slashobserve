function count(n: number, singular: string, plural: string): string {
  return `${n.toLocaleString()} ${n === 1 ? singular : plural}`
}

export function chatFooterText(
  chats: number,
  people: number | null,
  more: boolean
): string | null {
  if (chats === 0) return null
  const from =
    people === null ? "" : ` from ${count(people, "person", "people")}`
  const listed = count(chats, "chat", "chats")
  return more
    ? `${listed}${from} so far. Older chats load as you scroll.`
    : `${listed}${from}.`
}
