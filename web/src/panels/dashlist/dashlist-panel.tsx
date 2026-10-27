import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useMemo } from "react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"

import { PanelSkeleton } from "../panel-skeleton"
import type { StaticPanelRenderer } from "../types"
import { dashlistOptions, dashlistSearch } from "./dashlist-links"

const emptyText = "No dashboards match."

function Message({ children }: { children: string }) {
  return (
    <p className="m-0 flex size-full items-center justify-center p-3 text-center text-xs text-muted-foreground">
      {children}
    </p>
  )
}

const DashlistPanel: StaticPanelRenderer = ({ panel, vars, from, to }) => {
  const display = panel.type === "dashlist" ? panel.display : undefined
  const options = useMemo(() => dashlistOptions(display), [display])
  const { query, tags, limit } = options
  const list = useQuery({
    queryKey: [...dashboardKeys.list(), "panel", { query, tags, limit }],
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: {
            query: {
              q: query === "" ? undefined : query,
              tag: tags.length === 0 ? undefined : tags,
              limit,
            },
          },
          signal,
        })
        .then(unwrap),
    staleTime: 15_000,
  })
  const search = useMemo(
    () => dashlistSearch(options, { from, to }, vars),
    [options, from, to, vars]
  )

  if (list.isPending) return <PanelSkeleton />
  if (list.isError) return <Message>{list.error.message}</Message>
  const { dashboards } = list.data
  if (dashboards.length === 0) return <Message>{emptyText}</Message>
  return (
    <ul
      data-dashlist
      className="m-0 size-full list-none overflow-auto p-0 pb-2 text-[13px]"
    >
      {dashboards.map((dashboard) => (
        <li
          key={dashboard.id}
          className="flex min-w-0 flex-wrap items-baseline gap-x-2 px-3 py-1"
        >
          <Link
            to="/dashboards/$dashboardId"
            params={{ dashboardId: dashboard.id }}
            search={search}
            className="rounded-sm font-medium wrap-anywhere no-underline outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            {dashboard.title}
          </Link>
          {dashboard.tags.map((tag) => (
            <span
              key={tag}
              className="rounded-sm bg-muted px-1.5 text-xs text-muted-foreground"
            >
              {tag}
            </span>
          ))}
        </li>
      ))}
    </ul>
  )
}

export default DashlistPanel
