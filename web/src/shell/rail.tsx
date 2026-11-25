import { useRef, type ReactNode } from "react"
import { Link, useRouteContext, useRouterState } from "@tanstack/react-router"
import {
  BotIcon,
  ChartGanttIcon,
  ChartLineIcon,
  DatabaseIcon,
  HouseIcon,
  KeyIcon,
  LayoutDashboardIcon,
  LogsIcon,
  NotebookTextIcon,
  PanelLeftOpenIcon,
  ScrollTextIcon,
  SettingsIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"

import { Mark } from "@/components/brand/mark"
import { SourceFlyoutItems } from "@/datasources/source-menu"
import { useBrowsableSources } from "@/datasources/use-sources"
import { modChordLabel } from "@/lib/shortcuts"
import { DashboardsFlyoutItems } from "@/pins/pinned-dashboards"

import { AccountMenu } from "./account-menu"
import { AuditFlyoutItems } from "./audit-list"
import {
  countLabel,
  isNavItemActive,
  visibleNavItems,
  type NavItem,
  type NavPath,
} from "./nav"
import {
  useOpenObservationsPage,
  useUnreadOpenCount,
} from "./open-observations"
import { PaletteButton } from "./palette-button"
import { RailFlyout, RailTip, railItemClass } from "./rail-flyout"
import { SidebarSheet } from "./sidebar-sheet"
import { sidebarChordKey, sidebarKeyShortcuts } from "./sidebar-shortcut"
import { useSidebarStore } from "./sidebar-store"

export const navIcons: Record<NavPath, LucideIcon> = {
  "/": HouseIcon,
  "/observations": NotebookTextIcon,
  "/dashboards": LayoutDashboardIcon,
  "/logs": LogsIcon,
  "/traces": ChartGanttIcon,
  "/explore/metrics": ChartLineIcon,
  "/agents": BotIcon,
  "/datasources": DatabaseIcon,
  "/keys": KeyIcon,
  "/people": UsersIcon,
  "/audit": ScrollTextIcon,
  "/settings": SettingsIcon,
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
  const open = useOpenObservationsPage()
  const openCount = countLabel(open.observations.length, open.more)
  const unreadOpen = useUnreadOpenCount(open.observations)
  const unreadNote = unreadOpen > 0 ? `, ${unreadOpen} unread` : ""
  return {
    "/observations":
      openCount === null
        ? undefined
        : {
            hint: `${openCount} open${unreadNote}`,
            ariaLabel: `Observations, ${openCount} open${unreadNote}`,
            decoration:
              unreadOpen > 0 ? (
                <span
                  aria-hidden="true"
                  className="absolute top-1.5 right-1.5 size-2 rounded-full border-2 border-chrome bg-primary"
                />
              ) : null,
          },
  }
}

function ExpandButton({ forced }: { forced: boolean }) {
  const setCollapsed = useSidebarStore((state) => state.setCollapsed)
  const sheetOpen = useSidebarStore((state) => state.sheetOpen)
  const setSheetOpen = useSidebarStore((state) => state.setSheetOpen)
  const expandRef = useRef<HTMLButtonElement>(null)
  const expandHint = modChordLabel(navigator.userAgent, sidebarChordKey)
  const icon = (
    <PanelLeftOpenIcon
      aria-hidden="true"
      className="size-4"
      strokeWidth={1.8}
    />
  )

  if (!forced) {
    return (
      <RailTip label="Expand sidebar" hint={expandHint}>
        <button
          type="button"
          aria-label="Expand sidebar"
          aria-keyshortcuts={sidebarKeyShortcuts}
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
      <RailTip label="Expand sidebar" hint={expandHint}>
        <button
          ref={expandRef}
          type="button"
          aria-label="Expand sidebar"
          aria-keyshortcuts={sidebarKeyShortcuts}
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
  const logs = useBrowsableSources("logs").sources.length > 1
  const traces = useBrowsableSources("traces").sources.length > 1
  const metrics = useBrowsableSources("metrics").sources.length > 1
  const flyouts: Partial<Record<NavPath, ReactNode>> = {
    "/dashboards": <DashboardsFlyoutItems />,
    "/audit": <AuditFlyoutItems />,
    ...(logs ? { "/logs": <SourceFlyoutItems kind="logs" /> } : {}),
    ...(traces ? { "/traces": <SourceFlyoutItems kind="traces" /> } : {}),
    ...(metrics
      ? { "/explore/metrics": <SourceFlyoutItems kind="metrics" /> }
      : {}),
  }

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
