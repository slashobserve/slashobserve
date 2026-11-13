import { Link, useRouterState } from "@tanstack/react-router"

import { dashboardLink, dashboardPath } from "@/lib/links"

import { usePins } from "./queries"

const shown = 10

export function PinnedDashboards({
  collapsed,
  onNavigate,
}: {
  collapsed: boolean
  onNavigate?: () => void
}) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const pins = usePins().data
  if (collapsed || pins === undefined || pins.length === 0) return null

  return (
    <ul
      aria-label="Pinned dashboards"
      className="ml-[18px] flex flex-col gap-0.5 border-l pl-1.5"
    >
      {pins.slice(0, shown).map((pin) => (
        <li key={pin.id}>
          <Link
            {...dashboardLink(pin.id, pin.title)}
            onClick={onNavigate}
            title={pin.title}
            aria-current={
              pathname === dashboardPath(pin.id, pin.title) ||
              pathname === dashboardPath(pin.id)
                ? "page"
                : undefined
            }
            className="flex h-7 items-center rounded-control border border-transparent px-2 text-[13px] text-foreground no-underline outline-none hover:bg-background focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-background"
          >
            <span className="truncate">{pin.title}</span>
          </Link>
        </li>
      ))}
      {pins.length > shown ? (
        <li>
          <Link
            to="/dashboards"
            onClick={onNavigate}
            aria-label="More pinned dashboards"
            className="flex h-7 items-center rounded-control px-2 text-xs text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            More
          </Link>
        </li>
      ) : null}
    </ul>
  )
}
