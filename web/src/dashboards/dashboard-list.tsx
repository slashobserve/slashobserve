import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { ArrowUpRightIcon, SearchIcon } from "lucide-react"
import { useEffect, useId, useRef, useState } from "react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { madeByLabel } from "@/lib/origin"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import { DatasourceMenu, MadeByMenu } from "./list-filter-menus"
import {
  countText,
  hasFilters,
  toFilters,
  type ListSearch,
} from "./list-filters"

type DashboardSummary =
  paths["/dashboards"]["get"]["responses"][200]["content"]["application/json"]["dashboards"][number]

const pageSize = 50
const skeletonRows = 5
const searchDelayMs = 250
const maxQueryLength = 200

function useDashboardList(search: ListSearch) {
  const filters = toFilters(search)
  return useInfiniteQuery({
    queryKey: dashboardKeys.filtered(filters),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/dashboards", {
          params: {
            query: {
              limit: pageSize,
              cursor: pageParam,
              q: filters.q,
              made_by: filters.madeBy,
              datasource: filters.datasource,
            },
          },
          signal,
        })
        .then(unwrap),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
  })
}

function ListHeader() {
  return (
    <thead>
      <tr>
        <HeaderCell>Title</HeaderCell>
        <HeaderCell>Made by</HeaderCell>
        <HeaderCell>Datasources</HeaderCell>
        <HeaderCell>Open comments</HeaderCell>
        <HeaderCell>Version</HeaderCell>
        <HeaderCell className="text-right">Updated</HeaderCell>
      </tr>
    </thead>
  )
}

function DashboardRow({ dashboard }: { dashboard: DashboardSummary }) {
  return (
    <tr>
      <Cell>
        <Link
          to="/dashboards/$dashboardId"
          params={{ dashboardId: dashboard.id }}
          target="_blank"
          rel="noopener"
          className="inline-flex items-center gap-1 rounded-sm font-medium wrap-anywhere no-underline outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          {dashboard.title}
          <ArrowUpRightIcon
            aria-hidden="true"
            className="size-3.5 flex-none text-muted-foreground"
          />
          <span className="sr-only"> (opens in a new tab)</span>
        </Link>
        {dashboard.description ? (
          <div className="text-xs wrap-anywhere text-muted-foreground">
            {dashboard.description}
          </div>
        ) : null}
      </Cell>
      <Cell className="min-w-40 text-muted-foreground">
        {madeByLabel(dashboard.creator)}
      </Cell>
      <Cell>
        {dashboard.datasource_uids.length > 0 ? (
          <span className="font-mono text-xs">
            {dashboard.datasource_uids.join(", ")}
          </span>
        ) : (
          <span className="text-muted-foreground">None</span>
        )}
      </Cell>
      <Cell className="tabular-nums">
        {dashboard.open_comment_count > 0
          ? dashboard.open_comment_count
          : "None"}
      </Cell>
      <Cell className="tabular-nums">v{dashboard.latest_version}</Cell>
      <Cell className="text-right whitespace-nowrap text-muted-foreground">
        <RelativeTime iso={dashboard.updated_at} />
      </Cell>
    </tr>
  )
}

function SkeletonRows() {
  return Array.from({ length: skeletonRows }, (_, index) => (
    <tr key={index}>
      <Cell>
        <Skeleton className="h-5 w-48 max-w-full" />
      </Cell>
      <Cell>
        <Skeleton className="h-5 w-28" />
      </Cell>
      <Cell>
        <Skeleton className="h-5 w-16" />
      </Cell>
      <Cell>
        <Skeleton className="h-5 w-10" />
      </Cell>
      <Cell>
        <Skeleton className="h-5 w-8" />
      </Cell>
      <Cell>
        <Skeleton className="ml-auto h-5 w-16" />
      </Cell>
    </tr>
  ))
}

type FilterRowProps = {
  search: ListSearch
  onSearchChange: (next: ListSearch) => void
  total: number | undefined
}

function FilterRow({ search, onSearchChange, total }: FilterRowProps) {
  const [text, setText] = useState(search.q ?? "")
  const emitted = useRef(search.q)
  const typed = text.trim() === "" ? undefined : text.trim()

  useEffect(() => {
    if (search.q === emitted.current) return
    emitted.current = search.q
    setText(search.q ?? "")
  }, [search.q])

  useEffect(() => {
    if (typed === search.q) return
    const timer = setTimeout(() => {
      emitted.current = typed
      onSearchChange({ ...search, q: typed })
    }, searchDelayMs)
    return () => clearTimeout(timer)
  }, [typed, search, onSearchChange])

  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <label className="flex h-8 w-[280px] max-w-full items-center gap-2 rounded-control border bg-background px-2.5 focus-within:border-ring focus-within:ring-2 focus-within:ring-ring">
        <SearchIcon
          aria-hidden="true"
          className="size-3.5 flex-none text-muted-foreground"
        />
        <span className="sr-only">Filter dashboards</span>
        <input
          type="text"
          value={text}
          maxLength={maxQueryLength}
          autoComplete="off"
          placeholder="Filter by title"
          onChange={(event) => setText(event.target.value)}
          className="min-w-0 flex-1 border-0 bg-transparent text-[13px] outline-none placeholder:text-muted-foreground"
        />
      </label>
      <MadeByMenu
        value={search.made_by}
        onChange={(made_by) => onSearchChange({ ...search, made_by })}
      />
      <DatasourceMenu
        value={search.datasource}
        onChange={(datasource) => onSearchChange({ ...search, datasource })}
      />
      {total !== undefined ? (
        <span role="status" className="ml-auto text-xs text-muted-foreground">
          {countText(total)}
        </span>
      ) : null}
    </div>
  )
}

type Props = {
  labelledBy: string
  search: ListSearch
  onSearchChange: (next: ListSearch) => void
}

export function DashboardList({ labelledBy, search, onSearchChange }: Props) {
  const query = useDashboardList(search)
  const dashboards = query.data?.pages.flatMap((page) => page.dashboards) ?? []
  const empty =
    query.data !== undefined &&
    !query.isPlaceholderData &&
    dashboards.length === 0
  const noDashboards = empty && !hasFilters(search)
  const showTable = query.isPending || dashboards.length > 0
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
      {noDashboards ? null : (
        <FilterRow
          search={search}
          onSearchChange={onSearchChange}
          total={query.data?.pages[0].total_count}
        />
      )}
      {query.isError && !loadMoreFailed ? (
        <div className="mb-4">
          <LoadProblem
            title="Could not load dashboards"
            detail={loadProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        </div>
      ) : null}
      {empty ? (
        <section className="flex flex-col items-center gap-2 py-16 text-center">
          {noDashboards ? (
            <>
              <h2 className="text-lg font-semibold">No dashboards yet</h2>
              <p className="text-sm text-muted-foreground">
                Ask for one in the chat and it shows up here.
              </p>
              <Link
                to="/"
                className={cn(buttonVariants({ variant: "outline" }))}
              >
                Open the chat
              </Link>
            </>
          ) : (
            <>
              <h2 className="text-lg font-semibold">
                No dashboards match these filters.
              </h2>
              <Button variant="outline" onClick={() => onSearchChange({})}>
                Clear filters
              </Button>
            </>
          )}
        </section>
      ) : null}
      {showTable ? (
        <>
          <TableCard minWidth={760} labelledBy={labelledBy}>
            <ListHeader />
            <tbody ref={rows} aria-busy={query.isPending}>
              {query.isPending ? (
                <SkeletonRows />
              ) : (
                dashboards.map((dashboard) => (
                  <DashboardRow key={dashboard.id} dashboard={dashboard} />
                ))
              )}
            </tbody>
          </TableCard>
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
                className="text-foreground"
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
