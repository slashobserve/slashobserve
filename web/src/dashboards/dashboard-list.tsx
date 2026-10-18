import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

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
        <TableHead className="w-full">Title</TableHead>
        <TableHead>Updated</TableHead>
        <TableHead className="text-right">Version</TableHead>
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
            className="font-medium underline-offset-4 hover:underline focus-visible:underline"
          >
            {dashboard.title}
          </Link>
          {dashboard.tags.map((tag) => (
            <Badge key={tag} variant="outline">
              {tag}
            </Badge>
          ))}
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">
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

export function DashboardList() {
  const query = useDashboardList()
  const dashboards = query.data?.pages.flatMap((page) => page.dashboards) ?? []
  const empty = query.isSuccess && dashboards.length === 0
  const showTable = query.isPending || (query.data !== undefined && !empty)
  const retry = () =>
    void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch())

  return (
    <main className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex flex-col gap-1">
        <h1 className="text-2xl font-semibold">Dashboards</h1>
        <p className="text-sm text-muted-foreground">
          Every dashboard, most recently updated first.
        </p>
      </header>
      {query.isError ? (
        <Alert variant="destructive" className="mb-4">
          <AlertTitle>Could not load dashboards</AlertTitle>
          <AlertDescription>
            <p>{query.error.message}</p>
            <Button variant="outline" size="sm" onClick={retry}>
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
          <Table aria-busy={query.isPending}>
            <ListHeader />
            <TableBody>
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
            <div className="mt-4 flex justify-center">
              <Button
                variant="outline"
                disabled={query.isFetchingNextPage}
                aria-busy={query.isFetchingNextPage}
                onClick={() => void query.fetchNextPage()}
              >
                Load more
              </Button>
            </div>
          ) : null}
        </>
      ) : null}
    </main>
  )
}
