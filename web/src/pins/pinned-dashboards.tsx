import { Link } from "@tanstack/react-router"

import {
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuLinkItem,
} from "@/components/ui/dropdown-menu"
import { dashboardLink, dashboardPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { railFlyoutItemClass, railFlyoutLabelClass } from "@/shell/rail-flyout"
import type { SubNavRow } from "@/shell/nav"
import { SubNavList, subNavLinkClass, subNavMoreClass } from "@/shell/sub-nav"

import type { Pin } from "./pins"
import { usePins } from "./queries"

const shown = 10

export function useVisiblePins() {
  const pins = usePins().data ?? []
  return { pins: pins.slice(0, shown), more: pins.length > shown }
}

export function pinRows(pins: readonly Pin[]): SubNavRow[] {
  return pins.map(({ id, title }) => ({
    id,
    paths: [dashboardPath(id, title), dashboardPath(id)],
  }))
}

export function PinnedDashboards({
  pins,
  more,
  currentId,
  onNavigate,
}: {
  pins: readonly Pin[]
  more: boolean
  currentId: string | null
  onNavigate?: () => void
}) {
  if (pins.length === 0) return null

  return (
    <SubNavList label="Pinned dashboards">
      {pins.map((pin) => (
        <li key={pin.id}>
          <Link
            {...dashboardLink(pin.id, pin.title)}
            onClick={onNavigate}
            title={pin.title}
            aria-current={pin.id === currentId ? "page" : undefined}
            className={subNavLinkClass}
          >
            <span className="truncate">{pin.title}</span>
          </Link>
        </li>
      ))}
      {more ? (
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
