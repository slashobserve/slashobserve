import {
  Fragment,
  useEffect,
  useId,
  useRef,
  useState,
  type PointerEvent,
  type ReactNode,
} from "react"
import {
  Link,
  useLinkProps,
  useRouteContext,
  useRouterState,
} from "@tanstack/react-router"
import { PanelLeftCloseIcon } from "lucide-react"

import { Wordmark } from "@/components/brand/wordmark"
import { sourceRows } from "@/datasources/sources"
import { useListedSources } from "@/datasources/use-sources"
import { useInboxCount } from "@/inbox/queries"
import { cn } from "@/lib/utils"
import {
  PinnedDashboards,
  pinRows,
  useVisiblePins,
} from "@/pins/pinned-dashboards"

import { AccountMenu } from "./account-menu"
import {
  subNavState,
  unreadBadge,
  visibleNavItems,
  type NavItem,
  type NavPath,
  type ParentState,
  type SubNavRow,
} from "./nav"
import {
  OpenObservations,
  observationRows,
  useOpenObservationsPage,
} from "./open-observations"
import { PaletteButton } from "./palette-button"
import { navIcons, RailContent } from "./rail"
import { useSidebarStore } from "./sidebar-store"
import { SourceList } from "./source-list"
import {
  clampSidebarWidth,
  keyboardWidth,
  sidebarCollapsedWidth,
  sidebarMaxWidth,
  sidebarMinWidth,
} from "./sidebar-width"
import { useFoldedByWidth } from "./use-folded"

const focusRing = "outline-none focus-visible:ring-2 focus-visible:ring-ring"

function HomeLink({ onNavigate }: { onNavigate?: () => void }) {
  const linkProps = useLinkProps({ to: "/" })
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
        "flex min-h-[34px] items-center rounded-control px-2.5 no-underline",
        focusRing
      )}
    >
      <Wordmark size={17} />
    </a>
  )
}

function UnreadMark({ badge }: { badge: string }) {
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
  state,
  unread,
  onNavigate,
}: {
  item: NavItem
  state: ParentState
  unread: string | null
  onNavigate?: () => void
}) {
  const Icon = navIcons[item.to]
  return (
    <Link
      to={item.to}
      onClick={onNavigate}
      activeOptions={{ exact: true }}
      aria-current={state === "current" ? "page" : undefined}
      aria-label={
        unread === null ? undefined : `${item.label}, ${unread} unread`
      }
      className={cn(
        "relative flex h-8 items-center gap-2.5 rounded-control border border-transparent px-2.5 text-sm text-foreground no-underline hover:bg-nav-active aria-[current=page]:bg-nav-active",
        focusRing
      )}
    >
      <Icon aria-hidden="true" className="size-4 flex-none" strokeWidth={1.8} />
      <span
        className={cn(
          "truncate",
          state === "contains-current" && "font-semibold"
        )}
      >
        {item.label}
      </span>
      {unread === null ? null : <UnreadMark badge={unread} />}
    </Link>
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

function CollapseButton() {
  const setCollapsed = useSidebarStore((state) => state.setCollapsed)
  return (
    <button
      type="button"
      aria-label="Collapse sidebar"
      title="Collapse sidebar"
      aria-expanded={true}
      aria-controls="app-sidebar"
      onClick={() => setCollapsed(true)}
      className="grid size-8 flex-none place-items-center rounded-control text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
    >
      <PanelLeftCloseIcon
        aria-hidden="true"
        className="size-4"
        strokeWidth={1.8}
      />
    </button>
  )
}

export function SidebarContent({
  onNavigate,
  footerAction,
}: {
  onNavigate?: () => void
  footerAction?: ReactNode
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

  const openObservations = useOpenObservationsPage()
  const pinned = useVisiblePins()
  const logs = useListedSources("logs")
  const traces = useListedSources("traces")

  const subLists: Partial<
    Record<
      NavPath,
      {
        rows: readonly SubNavRow[]
        render: (currentId: string | null) => ReactNode
      }
    >
  > = {
    "/observations": {
      rows: observationRows(openObservations.observations),
      render: (currentId) => (
        <OpenObservations
          {...openObservations}
          currentId={currentId}
          onNavigate={onNavigate}
        />
      ),
    },
    "/dashboards": {
      rows: pinRows(pinned.pins),
      render: (currentId) => (
        <PinnedDashboards
          {...pinned}
          currentId={currentId}
          onNavigate={onNavigate}
        />
      ),
    },
    "/logs": {
      rows: sourceRows("logs", logs.shown),
      render: (currentId) => (
        <SourceList
          kind="logs"
          {...logs}
          currentId={currentId}
          onNavigate={onNavigate}
        />
      ),
    },
    "/traces": {
      rows: sourceRows("traces", traces.shown),
      render: (currentId) => (
        <SourceList
          kind="traces"
          {...traces}
          currentId={currentId}
          onNavigate={onNavigate}
        />
      ),
    },
  }

  function renderItems(list: readonly NavItem[]) {
    return list.map((item) => {
      const subList = subLists[item.to]
      const { parent, currentId } = subNavState(
        item,
        subList?.rows ?? [],
        pathname
      )
      return (
        <Fragment key={item.to}>
          <NavLink
            item={item}
            state={parent}
            unread={item.to === "/inbox" ? unreadInbox : null}
            onNavigate={onNavigate}
          />
          {subList?.render(currentId) ?? null}
        </Fragment>
      )
    })
  }

  return (
    <>
      <div className="mb-2.5 flex items-center">
        <HomeLink onNavigate={onNavigate} />
      </div>
      <div className="mb-3.5">
        <PaletteButton variant="sidebar" onNavigate={onNavigate} />
      </div>
      <div className="-m-[3px] flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-[3px]">
        {renderItems(main)}
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
      </div>
      <div className="flex items-center gap-1">
        <AccountMenu collapsed={false} />
        {footerAction}
      </div>
    </>
  )
}

export function Sidebar({ rail = false }: { rail?: boolean }) {
  const width = useSidebarStore((state) => state.width)
  const storedCollapsed = useSidebarStore((state) => state.collapsed)
  const folded = useFoldedByWidth()
  const forced = rail || folded
  const collapsed = forced || storedCollapsed
  const [dragWidth, setDragWidth] = useState<number | null>(null)
  const shown = collapsed ? sidebarCollapsedWidth : (dragWidth ?? width)

  return (
    <nav
      id="app-sidebar"
      aria-label="Main"
      className={cn(
        "relative flex flex-none flex-col narrow:hidden",
        collapsed ? "items-center gap-1 px-2 py-3" : "gap-0.5 pt-3.5 pb-3",
        rail && "sticky top-0 h-svh self-start bg-chrome"
      )}
      style={collapsed ? { width: shown } : { width: shown, paddingInline: 10 }}
    >
      {collapsed ? (
        <RailContent forced={forced} />
      ) : (
        <>
          <SidebarContent footerAction={<CollapseButton />} />
          <ResizeGrip shown={shown} onDrag={setDragWidth} />
        </>
      )}
    </nav>
  )
}
