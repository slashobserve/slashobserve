import { useEffect, useId, useMemo, useRef, type RefObject } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import type { components } from "@/api/schema.gen"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { myImportsQuery } from "@/import/record-queries"
import { importStatus } from "@/import/record-summary"

import { ImportLink } from "./import-link"

type ImportMeta = components["schemas"]["ImportMeta"]

export function ImportHistory({
  panelId,
  heading,
}: {
  panelId: string
  heading: RefObject<HTMLHeadingElement | null>
}) {
  const query = useInfiniteQuery(myImportsQuery())
  const imports = useMemo(() => {
    const seen = new Set<string>()
    const unique: ImportMeta[] = []
    for (const page of query.data?.pages ?? []) {
      for (const item of page.imports) {
        if (seen.has(item.id)) continue
        seen.add(item.id)
        unique.push(item)
      }
    }
    return unique
  }, [query.data])
  const loadMoreFailed = query.isFetchNextPageError
  const loadMoreErrorId = useId()
  const rows = useRef<HTMLUListElement>(null)
  const loadMoreButton = useRef<HTMLButtonElement>(null)
  const retryButton = useRef<HTMLButtonElement>(null)
  const focusFrom = useRef<number | null>(null)
  const rowCount = imports.length
  const fetching = query.isFetching

  useEffect(() => {
    const from = focusFrom.current
    if (fetching || from === null) return
    focusFrom.current = null
    const active = document.activeElement
    const onBody = !active || active === document.body
    const focusRow = (index: number) =>
      rows.current?.children[index]?.querySelector("a")?.focus()
    if (rowCount > from) {
      if (!onBody && active !== loadMoreButton.current) return
      focusRow(from)
    } else if (onBody) {
      if (rowCount > 0) focusRow(rowCount - 1)
      else (retryButton.current ?? heading.current)?.focus()
    }
  }, [rowCount, fetching, heading])

  const retry = () => {
    focusFrom.current = 0
    void query.refetch()
  }

  const loadMore = () => {
    focusFrom.current = rowCount
    void query.fetchNextPage()
  }

  if (query.isPending) {
    return (
      <div id={panelId} aria-busy="true" className="flex flex-col gap-2">
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className="h-14" />
        ))}
      </div>
    )
  }

  if (query.data === undefined) {
    return (
      <div id={panelId} className="flex flex-col items-start gap-3">
        <p className="text-[13px] text-muted-foreground">
          Could not load your imports.
        </p>
        <Button
          ref={retryButton}
          variant="outline"
          size="sm"
          disabled={query.isFetching}
          focusableWhenDisabled
          onClick={retry}
        >
          Retry
        </Button>
      </div>
    )
  }

  if (imports.length === 0) {
    return (
      <div id={panelId} className="flex flex-col items-start gap-2">
        <p className="text-[13px] text-muted-foreground">No imports yet.</p>
        <div className="-ml-2.5">
          <ImportLink />
        </div>
      </div>
    )
  }

  return (
    <div id={panelId} className="flex flex-col gap-3">
      <ul ref={rows} className="-mx-2 flex flex-col">
        {imports.map((item) => {
          const status = importStatus(item.counts)
          return (
            <li key={item.id}>
              <Link
                to="/dashboards/$dashboardId"
                params={{ dashboardId: item.dashboard_id }}
                className="flex w-full flex-col gap-1 rounded-control px-2 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="line-clamp-2 text-[13px] wrap-anywhere">
                  {item.title}
                </span>
                <span className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                  <StatusLabel tone={status.tone}>{status.text}</StatusLabel>
                  <span>
                    {item.counts.total}{" "}
                    {item.counts.total === 1 ? "panel" : "panels"}
                  </span>
                  <RelativeTime iso={item.created_at} />
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
      {query.hasNextPage ? (
        <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
          {loadMoreFailed ? (
            <p
              id={loadMoreErrorId}
              role="alert"
              className="text-xs text-destructive"
            >
              Could not load more imports.
            </p>
          ) : null}
          <Button
            ref={loadMoreButton}
            variant="outline"
            size="sm"
            disabled={query.isFetchingNextPage}
            focusableWhenDisabled
            aria-busy={query.isFetchingNextPage}
            aria-describedby={loadMoreFailed ? loadMoreErrorId : undefined}
            onClick={loadMore}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </div>
  )
}
