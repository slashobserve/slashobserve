import { useEffect, useId, useRef, useState, type PointerEvent } from "react"
import { Link, useLinkProps, useRouterState } from "@tanstack/react-router"
import {
  DatabaseIcon,
  KeyIcon,
  LayoutDashboardIcon,
  SettingsIcon,
  TerminalIcon,
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
import { cn } from "@/lib/utils"

import { AccountMenu } from "./account-menu"
import { isNavItemActive, navItems, type NavItem, type NavPath } from "./nav"
import { useSidebarStore } from "./sidebar-store"
import {
  clampSidebarWidth,
  keyboardWidth,
  sidebarCollapsedWidth,
  sidebarMaxWidth,
  sidebarMinWidth,
} from "./sidebar-width"

const icons: Record<NavPath, LucideIcon> = {
  "/": TerminalIcon,
  "/dashboards": LayoutDashboardIcon,
  "/datasources": DatabaseIcon,
  "/keys": KeyIcon,
  "/people": UsersIcon,
  "/settings": SettingsIcon,
}

const focusRing = "outline-none focus-visible:ring-2 focus-visible:ring-ring"

function HomeLink({ collapsed }: { collapsed: boolean }) {
  const linkProps = useLinkProps({ to: "/" })
  return (
    <a
      {...linkProps}
      aria-label="/observe home"
      aria-current={undefined}
      className={cn(
        "mb-3.5 flex min-h-[34px] items-center rounded-control px-2.5 no-underline",
        focusRing,
        collapsed && "justify-center px-0"
      )}
    >
      {collapsed ? <Mark size={20} variant="rail" /> : <Wordmark size={17} />}
    </a>
  )
}

function NavLink({
  item,
  active,
  collapsed,
}: {
  item: NavItem
  active: boolean
  collapsed: boolean
}) {
  const Icon = icons[item.to]
  const props = {
    to: item.to,
    activeOptions: { exact: true },
    "aria-current": active ? ("page" as const) : undefined,
    className: cn(
      "flex h-8 items-center gap-2.5 rounded-[7px] border border-transparent px-2.5 text-sm text-foreground no-underline hover:bg-background aria-[current=page]:bg-background",
      focusRing,
      collapsed && "justify-center px-0"
    ),
  }
  const content = (
    <>
      <Icon aria-hidden="true" className="size-4 flex-none" strokeWidth={1.8} />
      <span className={collapsed ? "sr-only" : "truncate"}>{item.label}</span>
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

export function Sidebar() {
  const width = useSidebarStore((state) => state.width)
  const collapsed = useSidebarStore((state) => state.collapsed)
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const [dragWidth, setDragWidth] = useState<number | null>(null)
  const workspaceId = useId()
  const shown = collapsed ? sidebarCollapsedWidth : (dragWidth ?? width)
  const main = navItems.filter((item) => item.group === "main")
  const workspace = navItems.filter((item) => item.group === "workspace")

  function renderItems(items: readonly NavItem[]) {
    return items.map((item) => (
      <NavLink
        key={item.to}
        item={item}
        active={isNavItemActive(item.to, pathname)}
        collapsed={collapsed}
      />
    ))
  }

  return (
    <nav
      id="app-sidebar"
      aria-label="Main"
      className="relative flex flex-none flex-col gap-0.5 pt-3.5 pb-3 narrow:hidden"
      style={{ width: shown, paddingInline: collapsed ? 6 : 10 }}
    >
      <HomeLink collapsed={collapsed} />
      <div className="-m-[3px] flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-[3px]">
        {renderItems(main)}
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
      {collapsed ? null : <ResizeGrip shown={shown} onDrag={setDragWidth} />}
    </nav>
  )
}
