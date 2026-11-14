import { useRef, useState } from "react"
import { Link, useRouterState } from "@tanstack/react-router"
import { InboxIcon, MenuIcon } from "lucide-react"

import { Wordmark } from "@/components/brand/wordmark"
import { PhoneSourceTitle } from "@/datasources/source-menu"
import { useRouteSource } from "@/datasources/use-sources"
import { useInboxCount } from "@/inbox/queries"
import { cn } from "@/lib/utils"

import { useCrumbs } from "./crumbs"
import { CountBadge } from "./count-badge"
import { unreadBadge } from "./nav"
import { PaletteButton } from "./palette-button"
import { phoneTitle } from "./phone-title"
import { SidebarSheet } from "./sidebar-sheet"

const iconButton =
  "grid size-11 flex-none place-items-center rounded-control text-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"

export function PhoneBar() {
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLButtonElement>(null)
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { crumbs } = useCrumbs()
  const title = phoneTitle(pathname, crumbs)
  const exploreKind = pathname.startsWith("/traces/") ? "traces" : "logs"
  const source = useRouteSource(exploreKind).current
  const unread = unreadBadge(useInboxCount())
  return (
    <header className="sticky top-0 z-10 hidden h-14 flex-none items-center gap-0.5 border-b bg-background px-1 narrow:flex">
      <button
        ref={menuRef}
        type="button"
        aria-label="Menu"
        aria-haspopup="dialog"
        aria-expanded={menuOpen}
        onClick={() => setMenuOpen(true)}
        className={iconButton}
      >
        <MenuIcon aria-hidden="true" className="size-5" />
      </button>
      <SidebarSheet
        open={menuOpen}
        onOpenChange={setMenuOpen}
        returnFocus={menuRef}
      />
      <div className="flex min-w-0 flex-1 items-center px-1">
        {title.kind === "wordmark" ? (
          <Link
            to="/"
            aria-label="/observe home"
            className="flex h-11 items-center rounded-control outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Wordmark size={17} />
          </Link>
        ) : source ? (
          <PhoneSourceTitle
            kind={exploreKind}
            label={title.label}
            source={source}
          />
        ) : (
          <div className="flex min-w-0 flex-col">
            <span className="truncate text-base leading-5 font-semibold">
              {title.label}
            </span>
          </div>
        )}
      </div>
      <PaletteButton variant="phone" />
      <Link
        to="/inbox"
        aria-label={unread === null ? "Inbox" : `Inbox, ${unread} unread`}
        className={cn(iconButton, "relative no-underline")}
      >
        <InboxIcon aria-hidden="true" className="size-5" />
        {unread === null ? null : (
          <CountBadge
            count={unread}
            className="top-1.5 right-1 border-background"
          />
        )}
      </Link>
    </header>
  )
}
