import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import {
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuLinkItem,
} from "@/components/ui/dropdown-menu"
import { dashboardsHealthQuery } from "@/health/queries"
import { dashboardLink, dashboardPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { railFlyoutItemClass, railFlyoutLabelClass } from "@/shell/rail-flyout"
import type { SubNavRow } from "@/shell/nav"
import {
  SubNavList,
  SubNavSkeleton,
  subNavLinkClass,
  subNavMoreClass,
  useReservedRows,
} from "@/shell/sub-nav"

import { pinProblemText } from "./pin-problems"
import type { Pin } from "./pins"
import { usePins } from "./queries"

const shown = 10

export function useVisiblePins() {
  const [expanded, setExpanded] = useState(false)
  const query = usePins()
  const pins = query.data ?? []
  return {
    pins: expanded ? pins : pins.slice(0, shown),
    more: pins.length > shown,
    pending: query.isPending,
    expanded,
    onToggle: () => setExpanded((open) => !open),
  }
}

function usePinHealth(pins: readonly Pin[]) {
  return useQuery(dashboardsHealthQuery(pins.map(({ id }) => id))).data
}

function ProblemText({ text }: { text: string | null }) {
  if (text === null) return null
  return (
    <span className="ml-auto flex-none text-xs text-muted-foreground">
      {text}
    </span>
  )
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
  expanded,
  onToggle,
  pending = false,
  currentId,
  onNavigate,
}: {
  pins: readonly Pin[]
  more: boolean
  expanded: boolean
  onToggle: () => void
  pending?: boolean
  currentId: string | null
  onNavigate?: (() => void) | undefined
}) {
  const health = usePinHealth(pins)
  const reserved = useReservedRows(
    "pins",
    pending,
    pins.length + (more ? 1 : 0)
  )
  if (pending) {
    return <SubNavSkeleton label="Pinned dashboards" rows={reserved} />
  }
  if (pins.length === 0) return null

  return (
    <SubNavList label="Pinned dashboards">
      {pins.map((pin) => {
        const text = pinProblemText(health?.get(pin.id))
        return (
          <li key={pin.id}>
            <Link
              {...dashboardLink(pin.id, pin.title)}
              onClick={onNavigate}
              title={text === null ? pin.title : `${pin.title}, ${text}`}
              aria-current={pin.id === currentId ? "page" : undefined}
              className={subNavLinkClass}
            >
              <span className="truncate">{pin.title}</span>
              <ProblemText text={text} />
            </Link>
          </li>
        )
      })}
      {more ? (
        <li>
          <button
            type="button"
            aria-expanded={expanded}
            onClick={onToggle}
            className={cn(subNavMoreClass, "w-full")}
          >
            {expanded ? "Show fewer pins" : "Show all pins"}
          </button>
        </li>
      ) : null}
    </SubNavList>
  )
}

export function DashboardsFlyoutItems() {
  const pins = usePins().data
  const shownPins = pins?.slice(0, shown) ?? []
  const health = usePinHealth(shownPins)
  return (
    <>
      <DropdownMenuLinkItem
        render={<Link to="/dashboards" />}
        className={cn(railFlyoutItemClass, "font-semibold")}
      >
        Dashboards
      </DropdownMenuLinkItem>
      {shownPins.length === 0 ? null : (
        <DropdownMenuGroup>
          <DropdownMenuLabel className={railFlyoutLabelClass}>
            Pinned
          </DropdownMenuLabel>
          {shownPins.map((pin) => {
            const text = pinProblemText(health?.get(pin.id))
            return (
              <DropdownMenuLinkItem
                key={pin.id}
                render={<Link {...dashboardLink(pin.id, pin.title)} />}
                title={text === null ? pin.title : `${pin.title}, ${text}`}
                className={railFlyoutItemClass}
              >
                <span className="truncate">{pin.title}</span>
                <ProblemText text={text} />
              </DropdownMenuLinkItem>
            )
          })}
        </DropdownMenuGroup>
      )}
    </>
  )
}
