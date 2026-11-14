import type { RequestSummary } from "@/observe/queries"

export type ChatDayGroup = {
  label: "Pinned" | "Today" | "This week" | "Earlier"
  requests: RequestSummary[]
}

const groupOrder: ChatDayGroup["label"][] = [
  "Pinned",
  "Today",
  "This week",
  "Earlier",
]

export function groupChatsByDay(
  requests: readonly RequestSummary[],
  now: Date,
  pinnedFirst: boolean
): ChatDayGroup[] {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const weekStart = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate() - 6
  )
  const byLabel = new Map<ChatDayGroup["label"], RequestSummary[]>()
  for (const request of requests) {
    const created = new Date(request.created_at)
    const label =
      pinnedFirst && request.pinned_at !== null
        ? "Pinned"
        : created >= today
          ? "Today"
          : created >= weekStart
            ? "This week"
            : "Earlier"
    byLabel.set(label, [...(byLabel.get(label) ?? []), request])
  }
  return groupOrder.flatMap((label) => {
    const grouped = byLabel.get(label)
    return grouped === undefined ? [] : [{ label, requests: grouped }]
  })
}
