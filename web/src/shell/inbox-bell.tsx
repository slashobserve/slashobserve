import { BellIcon } from "lucide-react"

import { useInboxPanelStore } from "@/inbox/inbox-panel-store"
import { useInboxCount } from "@/inbox/queries"
import { cn } from "@/lib/utils"

import { CountBadge } from "./count-badge"
import { unreadBadge } from "./nav"

function inboxBellLabel(unread: string | null): string {
  return unread === null ? "Inbox" : `Inbox, ${unread} unread`
}

export function InboxBell({
  place,
  className,
  badgeClassName,
}: {
  place: "top" | "phone"
  className: string
  badgeClassName: string
}) {
  const open = useInboxPanelStore((state) => state.open)
  const toggle = useInboxPanelStore((state) => state.toggle)
  const unread = unreadBadge(useInboxCount())
  return (
    <button
      type="button"
      data-inbox-bell={place}
      aria-label={inboxBellLabel(unread)}
      aria-expanded={open}
      onClick={toggle}
      className={cn("relative", className)}
    >
      <BellIcon aria-hidden="true" className="size-4" strokeWidth={1.8} />
      {unread === null ? null : (
        <CountBadge count={unread} className={badgeClassName} />
      )}
    </button>
  )
}
