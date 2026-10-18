import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useEffect, useId, useRef } from "react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatDateTime, formatRelative } from "@/lib/time"

type DashboardSummary =
  paths["/dashboards"]["get"]["responses"][200]["content"]["application/json"]["dashboards"][number]

const pageSize = 50
const skeletonRows = 5
const busyButton = "text-foreground"

function useDashboardList() {
  return useInfiniteQuery({
    queryKey: dashboardKeys.list(),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/dashboards", {
          params: { query: { limit: pageSize, cursor: pageParam } },
          signal,
        })
        .then(unwrap),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 15_000,
  })
}

function ListHeader() {
  return (
    <TableHeader>
      <TableRow className="hover:bg-transparent">
        <TableHead>Title</TableHead>
        <TableHead className="w-36 sm:w-64">Updated</TableHead>
        <TableHead className="w-16 text-right">Version</TableHead>
      </TableRow>
    </TableHeader>
  )
}

function DashboardRow({ dashboard }: { dashboard: DashboardSummary }) {
  return (
    <TableRow>
      <TableCell className="whitespace-normal">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <Link
            to="/dashboards/$dashboardId"
            params={{ dashboardId: dashboard.id }}
            className="rounded-sm font-medium wrap-anywhere underline-offset-4 outline-none hover:underline focus-visible:underline focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            {dashboard.title}
          </Link>
          {dashboard.tags.map((tag) => (
            <Badge key={tag} variant="outline" className="max-w-full">
              <span className="truncate" title={tag}>
                {tag}
              </span>
            </Badge>
          ))}
        </div>
      </TableCell>
      <TableCell className="whitespace-normal text-muted-foreground">
        <time
          dateTime={dashboard.updated_at}
          title={formatDateTime(dashboard.updated_at)}
        >
          {formatRelative(dashboard.updated_at)}
        </time>
        <span className="max-sm:block"> by {dashboard.updated_by.name}</span>
      </TableCell>
      <TableCell className="text-right tabular-nums">
        v{dashboard.latest_version}
      </TableCell>
    </TableRow>
  )
}

function SkeletonRows() {
  return Array.from({ length: skeletonRows }, (_, index) => (
    <TableRow key={index} className="hover:bg-transparent">
      <TableCell>
        <Skeleton className="h-5 w-48 max-w-full max-sm:w-32" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-5 w-24 sm:w-36" />
      </TableCell>
      <TableCell>
        <Skeleton className="ml-auto h-5 w-8" />
      </TableCell>
    </TableRow>
  ))
}

export function DashboardList({ labelledBy }: { labelledBy: string }) {
  const query = useDashboardList()
  const dashboards = query.data?.pages.flatMap((page) => page.dashboards) ?? []
  const empty = query.data !== undefined && dashboards.length === 0
  const showTable = query.isPending || (query.data !== undefined && !empty)
  const loadMoreFailed = query.isFetchNextPageError
  const loadMoreErrorId = useId()
  const rows = useRef<HTMLTableSectionElement>(null)
  const loadMoreButton = useRef<HTMLButtonElement>(null)
  const focusFrom = useRef<number | null>(null)
  const rowCount = dashboards.length

  const fetchingNextPage = query.isFetchingNextPage
  useEffect(() => {
    const from = focusFrom.current
    if (fetchingNextPage || from === null) return
    focusFrom.current = null
    const active = document.activeElement
    const onBody = !active || active === document.body
    if (rowCount > from) {
      if (!onBody && active !== loadMoreButton.current) return
      rows.current?.rows[from]?.querySelector("a")?.focus()
    } else if (onBody) {
      rows.current?.rows[rowCount - 1]?.querySelector("a")?.focus()
    }
  }, [rowCount, fetchingNextPage])

  const loadMore = () => {
    focusFrom.current = rowCount
    void query.fetchNextPage()
  }

  return (
    <>
      {query.isError && !loadMoreFailed ? (
        <Alert variant="destructive" className="mb-4">
          <AlertTitle>Could not load dashboards</AlertTitle>
          <AlertDescription>
            <p>{query.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              className={busyButton}
              disabled={query.isFetching}
              focusableWhenDisabled
              aria-busy={query.isFetching}
              onClick={() => void query.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {empty ? (
        <section className="flex flex-col items-center gap-2 py-16 text-center">
          <h2 className="text-lg font-semibold">No dashboards yet</h2>
          <p className="text-sm text-muted-foreground">
            Dashboards you or the agent create appear here.
          </p>
        </section>
      ) : null}
      {showTable ? (
        <>
          <Table
            className="table-fixed"
            aria-labelledby={labelledBy}
            aria-busy={query.isPending}
          >
            <ListHeader />
            <TableBody ref={rows}>
              {query.isPending ? (
                <SkeletonRows />
              ) : (
                dashboards.map((dashboard) => (
                  <DashboardRow key={dashboard.id} dashboard={dashboard} />
                ))
              )}
            </TableBody>
          </Table>
          {query.hasNextPage ? (
            <div className="mt-4 flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
              {loadMoreFailed ? (
                <p
                  id={loadMoreErrorId}
                  role="alert"
                  className="text-center text-sm text-destructive"
                >
                  Could not load more dashboards: {query.error?.message}
                </p>
              ) : null}
              <Button
                ref={loadMoreButton}
                variant="outline"
                className={busyButton}
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
        </>
      ) : null}
    </>
  )
}
