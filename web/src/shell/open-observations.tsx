import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useMemo } from "react"

import { unreadObservationIds } from "@/inbox/inbox-item"
import { useInboxForYou } from "@/inbox/queries"
import { observationLink, observationPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { observationsInfiniteQuery } from "@/observations/queries"

import { holdOrder } from "./held-order"
import type { SubNavRow } from "./nav"
import {
  SubNavList,
  SubNavSkeleton,
  subNavLinkClass,
  subNavMoreClass,
  useReservedRows,
} from "./sub-nav"
import { plainTitle } from "@/comments/plain-title"

const shown = 5
const staleTime = 30_000

export function useOpenObservationsPage() {
  const open = useInfiniteQuery({
    ...observationsInfiniteQuery({ state: "open" }, shown),
    staleTime,
  })
  const client = useQueryClient()
  const page = open.data?.pages[0]
  const observations = useMemo(
    () => holdOrder(client, page?.observations ?? []),
    [client, page]
  )
  return {
    observations,
    more: page?.next_cursor !== null && page?.next_cursor !== undefined,
    pending: open.isPending,
  }
}

function useUnreadIds() {
  return unreadObservationIds(useInboxForYou().data?.pages[0]?.items ?? [])
}

export function useUnreadOpenCount(
  observations: readonly OpenObservation[]
): number {
  const unread = useUnreadIds()
  return observations.filter(({ id }) => unread.has(id)).length
}

type OpenObservation = ReturnType<
  typeof useOpenObservationsPage
>["observations"][number]

export function observationRows(
  observations: readonly OpenObservation[]
): SubNavRow[] {
  return observations.map(({ id }) => ({ id, paths: [observationPath(id)] }))
}

export function OpenObservations({
  observations,
  more,
  pending = false,
  currentId,
  onNavigate,
}: {
  observations: readonly OpenObservation[]
  more: boolean
  pending?: boolean
  currentId: string | null
  onNavigate?: (() => void) | undefined
}) {
  const unread = useUnreadIds()
  const reserved = useReservedRows(
    "observations",
    pending,
    observations.length + (more ? 1 : 0)
  )
  if (pending) {
    return <SubNavSkeleton label="Open observations" rows={reserved} />
  }
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
              title={plainTitle(observation.title)}
              aria-current={observation.id === currentId ? "page" : undefined}
              className={subNavLinkClass}
            >
              <span
                aria-hidden="true"
                className="size-1.5 flex-none rounded-full bg-status-ok"
              />
              <span className={cn("truncate", isUnread && "font-semibold")}>
                {plainTitle(observation.title)}
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
