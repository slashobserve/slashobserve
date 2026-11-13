import { useRef, useState, type ReactNode } from "react"
import { Link, useRouteContext, useRouterState } from "@tanstack/react-router"
import {
  BotIcon,
  ChartGanttIcon,
  DatabaseIcon,
  HouseIcon,
  InboxIcon,
  LayoutDashboardIcon,
  LogsIcon,
  NotebookTextIcon,
  PanelLeftOpenIcon,
  SettingsIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"

import { Mark } from "@/components/brand/mark"
import { useInboxCount } from "@/inbox/queries"
import { modChordLabel } from "@/lib/shortcuts"
import { DashboardsFlyoutItems } from "@/pins/pinned-dashboards"

import { AccountMenu } from "./account-menu"
import { CountBadge } from "./count-badge"
import {
  countLabel,
  isNavItemActive,
  unreadBadge,
  visibleNavItems,
  type NavItem,
  type NavPath,
} from "./nav"
import { useOpenObservationsPage } from "./open-observations"
import { PaletteButton } from "./palette-button"
import { RailFlyout, RailTip, railItemClass } from "./rail-flyout"
import { SidebarSheet } from "./sidebar-sheet"
import { useSidebarStore } from "./sidebar-store"

export const navIcons: Record<NavPath, LucideIcon> = {
  "/": HouseIcon,
  "/inbox": InboxIcon,
  "/observations": NotebookTextIcon,
  "/dashboards": LayoutDashboardIcon,
  "/logs": LogsIcon,
  "/traces": ChartGanttIcon,
  "/agents": BotIcon,
  "/datasources": DatabaseIcon,
  "/people": UsersIcon,
  "/settings": SettingsIcon,
}

const flyouts: Partial<Record<NavPath, ReactNode>> = {
  "/dashboards": <DashboardsFlyoutItems />,
}

type RailBadge = {
  hint: string | null
  ariaLabel: string
  decoration: ReactNode
}

function RailLink({
  item,
  active,
  badge,
}: {
  item: NavItem
  active: boolean
  badge: RailBadge
}) {
  const Icon = navIcons[item.to]
  return (
    <RailTip label={item.label} hint={badge.hint}>
      <Link
        to={item.to}
        aria-label={badge.ariaLabel}
        aria-current={active ? "page" : undefined}
        className={railItemClass}
      >
        <Icon aria-hidden="true" className="size-4" strokeWidth={1.8} />
        {badge.decoration}
      </Link>
    </RailTip>
  )
}

function useRailBadges(): Partial<Record<NavPath, RailBadge>> {
  const unread = unreadBadge(useInboxCount())
  const open = useOpenObservationsPage()
  const openCount = countLabel(open.observations.length, open.more)
  return {
    "/inbox":
      unread === null
        ? undefined
        : {
            hint: `${unread} unread`,
            ariaLabel: `Inbox, ${unread} unread`,
            decoration: (
              <CountBadge
                count={unread}
                className="top-0.5 right-0 border-chrome"
              />
            ),
          },
    "/observations":
      openCount === null
        ? undefined
        : {
            hint: `${openCount} open`,
            ariaLabel: `Observations, ${openCount} open`,
            decoration: (
              <span
                aria-hidden="true"
                className="absolute top-1.5 right-1.5 size-2 rounded-full border-2 border-chrome bg-status-ok"
              />
            ),
          },
  }
}

function ExpandButton({ forced }: { forced: boolean }) {
  const setCollapsed = useSidebarStore((state) => state.setCollapsed)
  const [sheetOpen, setSheetOpen] = useState(false)
  const expandRef = useRef<HTMLButtonElement>(null)
  const icon = (
    <PanelLeftOpenIcon
      aria-hidden="true"
      className="size-4"
      strokeWidth={1.8}
    />
  )

  if (!forced) {
    return (
      <RailTip label="Expand sidebar">
        <button
          type="button"
          aria-label="Expand sidebar"
          aria-expanded={false}
          aria-controls="app-sidebar"
          onClick={() => setCollapsed(false)}
          className={railItemClass}
        >
          {icon}
        </button>
      </RailTip>
    )
  }
  return (
    <>
      <RailTip label="Expand sidebar">
        <button
          ref={expandRef}
          type="button"
          aria-label="Expand sidebar"
          aria-haspopup="dialog"
          aria-expanded={sheetOpen}
          onClick={() => setSheetOpen(true)}
          className={railItemClass}
        >
          {icon}
        </button>
      </RailTip>
      <SidebarSheet
        open={sheetOpen}
        onOpenChange={setSheetOpen}
        returnFocus={expandRef}
      />
    </>
  )
}

export function RailContent({ forced }: { forced: boolean }) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { principal } = useRouteContext({ from: "/_authed" })
  const items = visibleNavItems(principal.role)
  const main = items.filter((item) => item.group === "main")
  const workspace = items.filter((item) => item.group === "workspace")
  const badges = useRailBadges()

  function renderItems(list: readonly NavItem[]) {
    return list.map((item) => {
      const active = isNavItemActive(item.to, pathname)
      const flyout = flyouts[item.to]
      if (flyout !== undefined) {
        return (
          <RailFlyout
            key={item.to}
            label={item.label}
            icon={navIcons[item.to]}
            active={active}
          >
            {flyout}
          </RailFlyout>
        )
      }
      return (
        <RailLink
          key={item.to}
          item={item}
          active={active}
          badge={
            badges[item.to] ?? {
              hint: null,
              ariaLabel: item.label,
              decoration: null,
            }
          }
        />
      )
    })
  }

  return (
    <>
      <RailTip label="/observe">
        <Link
          to="/"
          aria-label="/observe home"
          className="grid size-9 place-items-center rounded-control outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Mark size={22} />
        </Link>
      </RailTip>
      <div className="mt-1.5 mb-2">
        <RailTip
          label="Search or jump to"
          hint={modChordLabel(navigator.userAgent, "K")}
        >
          <PaletteButton variant="rail" />
        </RailTip>
      </div>
      {renderItems(main)}
      <span aria-hidden="true" className="my-2 h-px w-5 bg-border" />
      {renderItems(workspace)}
      <div className="flex-1" />
      <ExpandButton forced={forced} />
      <AccountMenu collapsed />
    </>
  )
}
