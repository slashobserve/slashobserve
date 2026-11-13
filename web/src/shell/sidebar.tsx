import {
  Fragment,
  useEffect,
  useId,
  useRef,
  useState,
  type PointerEvent,
} from "react"
import {
  Link,
  useLinkProps,
  useRouteContext,
  useRouterState,
} from "@tanstack/react-router"
import {
  BotIcon,
  ChartGanttIcon,
  DatabaseIcon,
  InboxIcon,
  LayoutDashboardIcon,
  LogsIcon,
  NotebookTextIcon,
  SettingsIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react"

import { Mark } from "@/components/brand/mark"
import { Wordmark } from "@/components/brand/wordmark"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useInboxCount } from "@/inbox/queries"
import { cn } from "@/lib/utils"
import { PinnedDashboards } from "@/pins/pinned-dashboards"

import { AccountMenu } from "./account-menu"
import {
  isNavItemActive,
  unreadBadge,
  visibleNavItems,
  type NavItem,
  type NavPath,
} from "./nav"
import { NewMenu } from "./new-menu"
import { OpenObservations } from "./open-observations"
import { PaletteButton } from "./palette-button"
import { useSidebarStore } from "./sidebar-store"
import {
  clampSidebarWidth,
  keyboardWidth,
  sidebarCollapsedWidth,
  sidebarMaxWidth,
  sidebarMinWidth,
} from "./sidebar-width"
import { useFoldedByWidth } from "./use-folded"

const icons: Record<NavPath, LucideIcon> = {
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

const focusRing = "outline-none focus-visible:ring-2 focus-visible:ring-ring"

function HomeLink({
  collapsed,
  onNavigate,
}: {
  collapsed: boolean
  onNavigate?: () => void
}) {
  const linkProps = useLinkProps({ to: "/inbox" })
  return (
    <a
      {...linkProps}
      onClick={(event) => {
        linkProps.onClick?.(event)
        onNavigate?.()
      }}
      aria-label="/observe home"
      aria-current={undefined}
      className={cn(
        "flex items-center rounded-control no-underline",
        focusRing,
        collapsed ? "h-8 flex-1 justify-center" : "min-h-[34px] px-2.5"
      )}
    >
      {collapsed ? <Mark size={16} /> : <Wordmark size={17} />}
    </a>
  )
}

function UnreadMark({
  badge,
  collapsed,
}: {
  badge: string
  collapsed: boolean
}) {
  if (collapsed) {
    return (
      <span
        aria-hidden="true"
        className="absolute top-1.5 right-1.5 size-[7px] rounded-full bg-primary"
      />
    )
  }
  return (
    <span
      aria-hidden="true"
      className="ml-auto grid h-[18px] min-w-[18px] place-items-center rounded-full bg-primary px-1.5 text-[11px] font-semibold text-primary-foreground"
    >
      {badge}
    </span>
  )
}

function NavLink({
  item,
  active,
  collapsed,
  unread,
  onNavigate,
}: {
  item: NavItem
  active: boolean
  collapsed: boolean
  unread: string | null
  onNavigate?: () => void
}) {
  const Icon = icons[item.to]
  const props = {
    to: item.to,
    onClick: onNavigate,
    activeOptions: { exact: true },
    "aria-current": active ? ("page" as const) : undefined,
    "aria-label":
      unread === null ? undefined : `${item.label}, ${unread} unread`,
    className: cn(
      "relative flex h-8 items-center gap-2.5 rounded-control border border-transparent px-2.5 text-sm text-foreground no-underline hover:bg-background aria-[current=page]:bg-background",
      focusRing,
      collapsed && "justify-center px-0"
    ),
  }
  const content = (
    <>
      <Icon aria-hidden="true" className="size-4 flex-none" strokeWidth={1.8} />
      <span className={collapsed ? "sr-only" : "truncate"}>{item.label}</span>
      {unread === null ? null : (
        <UnreadMark badge={unread} collapsed={collapsed} />
      )}
    </>
  )
  if (!collapsed) return <Link {...props}>{content}</Link>
  return (
    <Tooltip>
      <TooltipTrigger render={<Link {...props} />}>{content}</TooltipTrigger>
      <TooltipContent side="right" role="tooltip">
        {item.label}
      </TooltipContent>
    </Tooltip>
  )
}

function ResizeGrip({
  shown,
  onDrag,
}: {
  shown: number
  onDrag: (width: number | null) => void
}) {
  const width = useSidebarStore((state) => state.width)
  const setWidth = useSidebarStore((state) => state.setWidth)
  const resetWidth = useSidebarStore((state) => state.resetWidth)
  const drag = useRef<{
    pointerId: number
    startX: number
    startWidth: number
    latest: number
  } | null>(null)

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    drag.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startWidth: width,
      latest: width,
    }
    document.body.style.cursor = "col-resize"
    document.body.style.userSelect = "none"
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const current = drag.current
    if (!current) return
    current.latest = clampSidebarWidth(
      current.startWidth + event.clientX - current.startX
    )
    onDrag(current.latest)
  }

  function endDrag(event?: PointerEvent<HTMLDivElement>) {
    const current = drag.current
    if (!current) return
    drag.current = null
    setWidth(current.latest)
    onDrag(null)
    const target = event?.currentTarget
    if (target?.hasPointerCapture(current.pointerId)) {
      target.releasePointerCapture(current.pointerId)
    }
    document.body.style.cursor = ""
    document.body.style.userSelect = ""
  }

  useEffect(
    () => () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.cursor = ""
      document.body.style.userSelect = ""
      onDrag(null)
    },
    [onDrag]
  )

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize sidebar"
      aria-controls="app-sidebar"
      aria-valuenow={shown}
      aria-valuemin={sidebarMinWidth}
      aria-valuemax={sidebarMaxWidth}
      aria-valuetext={`${shown} pixels`}
      tabIndex={0}
      title="Drag to resize, double-click to reset"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onLostPointerCapture={endDrag}
      onDoubleClick={resetWidth}
      onKeyDown={(event) => {
        const next = keyboardWidth(width, event.key, event.shiftKey)
        if (next === null) return
        event.preventDefault()
        setWidth(next)
      }}
      className={cn(
        "group absolute top-0 -right-1 bottom-0 z-10 grid w-2 cursor-col-resize touch-none place-items-center rounded-control",
        focusRing
      )}
    >
      <span className="h-9 w-1 rounded-[2px] bg-border group-hover:bg-muted-foreground/50" />
    </div>
  )
}

export function SidebarContent({
  collapsed,
  onNavigate,
}: {
  collapsed: boolean
  onNavigate?: () => void
}) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const workspaceId = useId()
  const { principal } = useRouteContext({ from: "/_authed" })
  const items = visibleNavItems(principal.role)
  const main = items.filter((item) => item.group === "main")
  const workspace = items.filter((item) => item.group === "workspace")
  const unreadInbox = unreadBadge(useInboxCount())

  function renderItems(items: readonly NavItem[]) {
    return items.map((item) => (
      <Fragment key={item.to}>
        <NavLink
          item={item}
          active={isNavItemActive(item.to, pathname)}
          collapsed={collapsed}
          unread={item.to === "/inbox" ? unreadInbox : null}
          onNavigate={onNavigate}
        />
        {item.to === "/dashboards" ? (
          <PinnedDashboards collapsed={collapsed} onNavigate={onNavigate} />
        ) : null}
      </Fragment>
    ))
  }

  return (
    <>
      <div className="mb-2.5 flex items-center justify-between">
        <HomeLink collapsed={collapsed} onNavigate={onNavigate} />
        <NewMenu collapsed={collapsed} onNavigate={onNavigate} />
      </div>
      <div className="mb-3.5">
        <PaletteButton
          variant="sidebar"
          collapsed={collapsed}
          onNavigate={onNavigate}
        />
      </div>
      <div className="-m-[3px] flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-[3px]">
        {renderItems(main)}
        <OpenObservations collapsed={collapsed} onNavigate={onNavigate} />
        {collapsed ? (
          <div
            role="group"
            aria-label="Workspace"
            className="flex flex-col gap-0.5"
          >
            <div aria-hidden="true" className="mx-1.5 my-2 h-px bg-border" />
            {renderItems(workspace)}
          </div>
        ) : (
          <div
            role="group"
            aria-labelledby={workspaceId}
            className="flex flex-col gap-0.5"
          >
            <div
              id={workspaceId}
              className="px-2.5 pt-3.5 pb-1.5 text-xs text-muted-foreground"
            >
              Workspace
            </div>
            {renderItems(workspace)}
          </div>
        )}
      </div>
      <AccountMenu collapsed={collapsed} />
    </>
  )
}

export function Sidebar({ rail = false }: { rail?: boolean }) {
  const width = useSidebarStore((state) => state.width)
  const storedCollapsed = useSidebarStore((state) => state.collapsed)
  const folded = useFoldedByWidth()
  const collapsed = rail || storedCollapsed || folded
  const [dragWidth, setDragWidth] = useState<number | null>(null)
  const shown = collapsed ? sidebarCollapsedWidth : (dragWidth ?? width)

  return (
    <nav
      id="app-sidebar"
      aria-label="Main"
      className={cn(
        "relative flex flex-none flex-col gap-0.5 pt-3.5 pb-3 narrow:hidden",
        rail && "sticky top-0 h-svh self-start bg-chrome"
      )}
      style={{ width: shown, paddingInline: collapsed ? 6 : 10 }}
    >
      <SidebarContent collapsed={collapsed} />
      {collapsed ? null : <ResizeGrip shown={shown} onDrag={setDragWidth} />}
    </nav>
  )
}
