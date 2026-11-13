import { Link, useRouterState } from "@tanstack/react-router"

import {
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuLinkItem,
} from "@/components/ui/dropdown-menu"
import { dashboardLink, dashboardPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { railFlyoutItemClass, railFlyoutLabelClass } from "@/shell/rail-flyout"
import { SubNavList, subNavLinkClass, subNavMoreClass } from "@/shell/sub-nav"

import { usePins } from "./queries"

const shown = 10

export function PinnedDashboards({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const pins = usePins().data
  if (pins === undefined || pins.length === 0) return null

  return (
    <SubNavList label="Pinned dashboards">
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
            className={subNavLinkClass}
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
            className={subNavMoreClass}
          >
            More
          </Link>
        </li>
      ) : null}
    </SubNavList>
  )
}

export function DashboardsFlyoutItems() {
  const pins = usePins().data
  return (
    <>
      <DropdownMenuLinkItem
        render={<Link to="/dashboards" />}
        className={cn(railFlyoutItemClass, "font-semibold")}
      >
        Dashboards
      </DropdownMenuLinkItem>
      {pins === undefined || pins.length === 0 ? null : (
        <DropdownMenuGroup>
          <DropdownMenuLabel className={railFlyoutLabelClass}>
            Pinned
          </DropdownMenuLabel>
          {pins.slice(0, shown).map((pin) => (
            <DropdownMenuLinkItem
              key={pin.id}
              render={<Link {...dashboardLink(pin.id, pin.title)} />}
              title={pin.title}
              className={railFlyoutItemClass}
            >
              <span className="truncate">{pin.title}</span>
            </DropdownMenuLinkItem>
          ))}
        </DropdownMenuGroup>
      )}
    </>
  )
}
