import { useInfiniteQuery } from "@tanstack/react-query"
import { Link, useRouterState } from "@tanstack/react-router"

import { unreadObservationIds } from "@/inbox/inbox-item"
import { useInboxForYou } from "@/inbox/queries"
import { observationLink, observationPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { observationsInfiniteQuery } from "@/observations/queries"

import { SubNavList, subNavLinkClass, subNavMoreClass } from "./sub-nav"

const shown = 5
const staleTime = 30_000

export function useOpenObservationsPage() {
  const open = useInfiniteQuery({
    ...observationsInfiniteQuery({ state: "open" }, shown),
    staleTime,
  })
  const page = open.data?.pages[0]
  return {
    observations: page?.observations ?? [],
    more: page?.next_cursor !== null && page?.next_cursor !== undefined,
  }
}

export function OpenObservations({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { observations, more } = useOpenObservationsPage()
  const unread = unreadObservationIds(
    useInboxForYou().data?.pages[0]?.items ?? []
  )
  if (observations.length === 0) return null

  return (
    <SubNavList label="Open observations">
      {observations.map((observation) => {
        const isUnread = unread.has(observation.id)
        return (
          <li key={observation.id}>
            <Link
              {...observationLink(observation.id)}
              onClick={onNavigate}
              title={observation.title}
              aria-current={
                pathname === observationPath(observation.id)
                  ? "page"
                  : undefined
              }
              className={subNavLinkClass}
            >
              <span
                aria-hidden="true"
                className="size-1.5 flex-none rounded-full bg-status-ok"
              />
              <span className={cn("truncate", isUnread && "font-semibold")}>
                {observation.title}
              </span>
              {isUnread ? <span className="sr-only">Unread</span> : null}
            </Link>
          </li>
        )
      })}
      {more ? (
        <li>
          <Link
            to="/observations"
            onClick={onNavigate}
            className={subNavMoreClass}
          >
            All observations
          </Link>
        </li>
      ) : null}
    </SubNavList>
  )
}
