import { useId } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link, useRouterState } from "@tanstack/react-router"

import { observationLink, observationPath } from "@/lib/links"
import { observationsInfiniteQuery } from "@/observations/queries"

const limit = 8
const staleTime = 30_000

export function OpenObservations({
  collapsed,
  onNavigate,
}: {
  collapsed: boolean
  onNavigate?: () => void
}) {
  const labelId = useId()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const open = useInfiniteQuery({
    ...observationsInfiniteQuery({ state: "open" }, limit),
    staleTime,
  })
  const page = open.data?.pages[0]
  if (collapsed || page === undefined || page.observations.length === 0) {
    return null
  }
  const count = `${page.observations.length}${page.next_cursor ? "+" : ""}`

  return (
    <div
      role="group"
      aria-labelledby={labelId}
      className="flex flex-col gap-0.5"
    >
      <div
        id={labelId}
        className="flex items-center justify-between px-2.5 pt-3.5 pb-1.5 text-xs text-muted-foreground"
      >
        <span>Open observations</span>
        <span>{count}</span>
      </div>
      <ul className="flex flex-col gap-0.5">
        {page.observations.map((observation) => (
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
              className="flex h-8 items-center gap-2.5 rounded-control border border-transparent pr-2.5 pl-4 text-[13px] text-foreground no-underline outline-none hover:bg-background focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-background"
            >
              <span
                aria-hidden="true"
                className="size-[7px] flex-none rounded-full bg-border"
              />
              <span className="truncate">{observation.title}</span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}
