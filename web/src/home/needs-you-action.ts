import { itemLink, type InboxItem, type ItemLink } from "@/inbox/inbox-item"

export type NeedsYouAction =
  | { kind: "link"; label: "Review" | "Reply" | "Open"; to: ItemLink }
  | { kind: "retry"; requestId: string }

function linkAction(
  item: InboxItem,
  label: "Review" | "Reply" | "Open"
): NeedsYouAction | null {
  const to = itemLink(item)
  return to === null ? null : { kind: "link", label, to }
}

export function needsYouAction(item: InboxItem): NeedsYouAction | null {
  switch (item.reason) {
    case "proposal":
    case "requested_block":
      return linkAction(item, "Review")
    case "mention":
    case "reply":
      return linkAction(item, "Reply")
    case "repaired":
      return linkAction(item, "Open")
    case "run_failed":
      return item.target.kind === "run"
        ? { kind: "retry", requestId: item.target.id }
        : null
    case null:
      return null
  }
}
