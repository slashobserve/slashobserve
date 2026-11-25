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
  className,
  badgeClassName,
}: {
  className: string
  badgeClassName: string
}) {
  const open = useInboxPanelStore((state) => state.open)
  const toggle = useInboxPanelStore((state) => state.toggle)
  const unread = unreadBadge(useInboxCount())
  return (
    <button
      type="button"
      data-inbox-bell=""
      aria-label={inboxBellLabel(unread)}
      aria-expanded={open}
      onClick={(event) => toggle(event.currentTarget)}
      className={cn("relative", className)}
    >
      <BellIcon aria-hidden="true" className="size-4" strokeWidth={1.8} />
      {unread === null ? null : (
        <CountBadge count={unread} className={badgeClassName} />
      )}
    </button>
  )
}

export function TopRowInboxBell() {
  return (
    <InboxBell
      className="grid size-8 flex-none place-items-center rounded-control text-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted narrow:hidden"
      badgeClassName="-top-0.5 -right-1 border-background"
    />
  )
}
