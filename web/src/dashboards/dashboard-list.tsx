import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useTable, type Table } from "@tanstack/react-table"
import { SearchIcon } from "lucide-react"
import { useEffect, useId, useMemo, useRef, useState } from "react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import {
  dashboardColumns,
  listFeatures,
  type DashboardSummary,
} from "./dashboard-columns"
import { DatasourceMenu, MadeByMenu, TagsMenu } from "./list-filter-menus"
import {
  clampQuery,
  countText,
  hasFilters,
  toFilters,
  type ListSearch,
} from "./list-filters"

const pageSize = 50
const skeletonRows = 5
const searchDelayMs = 250
const emptyList: DashboardSummary[] = []
const dashboardRowId = (dashboard: DashboardSummary) => dashboard.id

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
              tag: filters.tags,
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

type ListTable = Table<typeof listFeatures, DashboardSummary>

function SkeletonRows({ table }: { table: ListTable }) {
  return Array.from({ length: skeletonRows }, (_, index) => (
    <tr key={index}>
      {table.getAllLeafColumns().map((column) => (
        <Cell key={column.id}>
          <Skeleton className={column.columnDef.meta?.skeletonClassName} />
        </Cell>
      ))}
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
          autoComplete="off"
          placeholder="Filter by title"
          onChange={(event) => setText(clampQuery(event.target.value))}
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
      <TagsMenu
        value={search.tag}
        onChange={(tag) => onSearchChange({ ...search, tag })}
      />
      <span role="status" className="ml-auto text-xs text-muted-foreground">
        {total !== undefined ? countText(total) : null}
      </span>
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
  const pages = query.data?.pages
  const dashboards = useMemo(
    () => pages?.flatMap((page) => page.dashboards) ?? emptyList,
    [pages]
  )
  const table = useTable({
    features: listFeatures,
    columns: dashboardColumns,
    data: dashboards,
    getRowId: dashboardRowId,
  })
  const empty = query.data !== undefined && dashboards.length === 0
  const noDashboards = empty && !query.isPlaceholderData && !hasFilters(search)
  const [resetKey, setResetKey] = useState(0)
  const showTable = query.isPending || dashboards.length > 0
  const loadMoreFailed = query.isFetchNextPageError
  const loadMoreErrorId = useId()
  const rows = useRef<HTMLTableSectionElement>(null)
  const loadMoreButton = useRef<HTMLButtonElement>(null)
  const focusFrom = useRef<number | null>(null)
  const rowCount = dashboards.length

  const { q, made_by: madeBy, datasource } = search
  const tags = search.tag?.join(",")
  useEffect(() => {
    focusFrom.current = null
  }, [q, madeBy, datasource, tags])

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
          key={resetKey}
          search={search}
          onSearchChange={onSearchChange}
          total={
            query.isPlaceholderData
              ? undefined
              : query.data?.pages[0].total_count
          }
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
              <div className="flex gap-2">
                <Link
                  to="/"
                  className={cn(buttonVariants({ variant: "outline" }))}
                >
                  Open the chat
                </Link>
                <Link
                  to="/import"
                  className={cn(buttonVariants({ variant: "outline" }))}
                >
                  Import from Grafana
                </Link>
              </div>
            </>
          ) : (
            <>
              <h2 className="text-lg font-semibold">
                No dashboards match these filters.
              </h2>
              <Button
                variant="outline"
                onClick={() => {
                  setResetKey((key) => key + 1)
                  onSearchChange({})
                }}
              >
                Clear filters
              </Button>
            </>
          )}
        </section>
      ) : null}
      {showTable ? (
        <>
          <TableCard minWidth={760} labelledBy={labelledBy}>
            <thead>
              {table.getHeaderGroups().map((group) => (
                <tr key={group.id}>
                  {group.headers.map((header) => (
                    <HeaderCell
                      key={header.id}
                      className={header.column.columnDef.meta?.headerClassName}
                    >
                      <table.FlexRender header={header} />
                    </HeaderCell>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody
              ref={rows}
              aria-busy={query.isPending || query.isPlaceholderData}
            >
              {query.isPending ? (
                <SkeletonRows table={table} />
              ) : (
                table.getRowModel().rows.map((row) => (
                  <tr key={row.id} className="group/row">
                    {row.getAllCells().map((cell) => (
                      <Cell
                        key={cell.id}
                        className={cell.column.columnDef.meta?.cellClassName}
                      >
                        <table.FlexRender cell={cell} />
                      </Cell>
                    ))}
                  </tr>
                ))
              )}
            </tbody>
          </TableCard>
          {query.hasNextPage && !query.isPlaceholderData ? (
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
