import type { InboxFilter, InboxView } from "@/api/keys"

export const inboxViews: readonly { value: InboxView; label: string }[] = [
  { value: "for_you", label: "For you" },
  { value: "everything", label: "Everything" },
  { value: "proposals", label: "Proposals" },
  { value: "mentions", label: "Mentions" },
]

export const inboxFilters: readonly { value: InboxFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "unread", label: "Unread" },
  { value: "done", label: "Done" },
]

export function isInboxFilter(value: unknown): value is InboxFilter {
  return inboxFilters.some((filter) => filter.value === value)
}

export function isInboxView(value: unknown): value is InboxView {
  return inboxViews.some((view) => view.value === value)
}
