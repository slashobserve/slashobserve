import type { InboxFilter, InboxView } from "@/api/keys"

export const inboxViews: readonly { value: InboxView; label: string }[] = [
  { value: "for_you", label: "For you" },
  { value: "proposals", label: "Proposals" },
  { value: "mentions", label: "Mentions" },
  { value: "everything", label: "Everything" },
]

export const inboxFilters: readonly { value: InboxFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "unread", label: "Unread" },
  { value: "done", label: "Done" },
]

function isInboxFilter(value: unknown): value is InboxFilter {
  return inboxFilters.some((filter) => filter.value === value)
}

function isInboxView(value: unknown): value is InboxView {
  return inboxViews.some((view) => view.value === value)
}

export function inboxChoice(search: Record<string, unknown>): {
  view: InboxView
  filter: InboxFilter
} {
  return {
    view: isInboxView(search.view) ? search.view : "for_you",
    filter: isInboxFilter(search.filter) ? search.filter : "open",
  }
}
