import type { InboxView } from "@/api/keys"

export const inboxViews: readonly { value: InboxView; label: string }[] = [
  { value: "for_you", label: "For you" },
  { value: "everything", label: "Everything" },
  { value: "proposals", label: "Proposals" },
  { value: "mentions", label: "Mentions" },
]

export function isInboxView(value: unknown): value is InboxView {
  return inboxViews.some((view) => view.value === value)
}
