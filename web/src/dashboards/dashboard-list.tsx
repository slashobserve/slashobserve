import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
} from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { useTable } from "@tanstack/react-table"
import {
  ChevronDownIcon,
  ChevronRightIcon,
  LockIcon,
  SearchIcon,
  UsersIcon,
} from "lucide-react"
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { foldersQuery, type Folder } from "@/folders/queries"
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import {
  actionsColumn,
  dashboardColumns,
  listFeatures,
  type DashboardSummary,
} from "./dashboard-columns"
import {
  accessSummary,
  dashboardCountText,
  groupByFolder,
  listSummary,
  type FolderSection,
} from "./folder-groups"
import {
  DatasourceMenu,
  FolderMenu,
  MadeByMenu,
  TagsMenu,
} from "./list-filter-menus"
import {
  clampQuery,
  hasFilters,
  toFilters,
  type ListSearch,
} from "./list-filters"

const pageSize = 50
const skeletonRows = 5
const searchDelayMs = 250
const emptyList: DashboardSummary[] = []
const listColumns = [...dashboardColumns, actionsColumn]
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
              folder: filters.folder,
              order: "folder",
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

function useListTable(data: DashboardSummary[]) {
  return useTable({
    features: listFeatures,
    columns: listColumns,
    data,
    getRowId: dashboardRowId,
  })
}

type ListTable = ReturnType<typeof useListTable>

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
  summary: string | undefined
}

function FilterRow({ search, onSearchChange, summary }: FilterRowProps) {
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
      <FolderMenu
        value={search.folder}
        onChange={(folder) => onSearchChange({ ...search, folder })}
      />
      <TagsMenu
        value={search.tag}
        onChange={(tag) => onSearchChange({ ...search, tag })}
      />
      <span role="status" className="ml-auto text-xs text-muted-foreground">
        {summary}
      </span>
    </div>
  )
}

function FolderHeaderRow({
  folder,
  collapsed,
  columnCount,
  onToggle,
}: {
  folder: Folder
  collapsed: boolean
  columnCount: number
  onToggle: () => void
}) {
  const AccessIcon = folder.restricted ? LockIcon : UsersIcon
  return (
    <tr>
      <th
        scope="rowgroup"
        colSpan={columnCount}
        className="border-b px-3.5 py-2.5 text-left font-normal"
      >
        <span className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-expanded={!collapsed}
            aria-label={`${collapsed ? "Expand" : "Collapse"} ${folder.title}`}
            onClick={onToggle}
          >
            {collapsed ? (
              <ChevronRightIcon aria-hidden="true" />
            ) : (
              <ChevronDownIcon aria-hidden="true" />
            )}
          </Button>
          <span className="font-semibold wrap-anywhere">{folder.title}</span>
          <span className="text-xs text-muted-foreground">
            {dashboardCountText(folder.dashboard_count)}
          </span>
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <AccessIcon aria-hidden="true" className="size-3.5 flex-none" />
            {accessSummary(folder)}
          </span>
          {folder.permissions.manage ? (
            <Link
              to="/folders/$folderId"
              params={{ folderId: folder.id }}
              className="ml-auto rounded-sm text-xs outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              Folder settings
            </Link>
          ) : null}
        </span>
      </th>
    </tr>
  )
}

function sectionRows(
  sections: FolderSection[],
  collapsed: ReadonlySet<string>,
  toggle: (folderId: string) => void,
  table: ListTable
): ReactNode[] {
  const columnCount = table.getAllLeafColumns().length
  const tableRows = new Map(
    table.getRowModel().rows.map((row) => [row.id, row])
  )
  return sections.flatMap((section) => {
    const isCollapsed = collapsed.has(section.folder.id)
    const header = (
      <FolderHeaderRow
        key={`folder:${section.folder.id}`}
        folder={section.folder}
        collapsed={isCollapsed}
        columnCount={columnCount}
        onToggle={() => toggle(section.folder.id)}
      />
    )
    if (isCollapsed) return [header]
    if (section.empty) {
      return [
        header,
        <tr key={`empty:${section.folder.id}`}>
          <Cell colSpan={columnCount} className="pl-11 text-muted-foreground">
            No dashboards yet
          </Cell>
        </tr>,
      ]
    }
    return [
      header,
      ...section.rows.flatMap((item) => {
        const row = tableRows.get(item.id)
        if (row === undefined) return []
        return [
          <tr key={row.id} data-dashboard-id={row.id} className="group/row">
            {row.getAllCells().map((cell) => (
              <Cell
                key={cell.id}
                className={cell.column.columnDef.meta?.cellClassName}
              >
                <table.FlexRender cell={cell} />
              </Cell>
            ))}
          </tr>,
        ]
      }),
    ]
  })
}

type Props = {
  labelledBy: string
  search: ListSearch
  onSearchChange: (next: ListSearch) => void
}

export function DashboardList({ labelledBy, search, onSearchChange }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const query = useDashboardList(search)
  const folderResult = useQuery(foldersQuery())
  const pages = query.data?.pages
  const dashboards = useMemo(
    () => pages?.flatMap((page) => page.dashboards) ?? emptyList,
    [pages]
  )
  const table = useListTable(dashboards)
  const folders = folderResult.data
  const filtered = hasFilters(search)
  const hasNextPage = query.hasNextPage
  const loaded = query.data !== undefined
  const sections = useMemo(
    () =>
      loaded && folders !== undefined
        ? groupByFolder(folders, dashboards, hasNextPage, filtered)
        : null,
    [loaded, folders, dashboards, hasNextPage, filtered]
  )
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const noDashboards =
    folders !== undefined &&
    loaded &&
    dashboards.length === 0 &&
    !query.isPlaceholderData &&
    !filtered &&
    folders.length <= 1
  const empty = sections !== null && (noDashboards || sections.length === 0)
  const [resetKey, setResetKey] = useState(0)
  const pending = query.isPending || folderResult.isPending
  const showTable = pending || (sections !== null && !empty)
  const loadMoreFailed = query.isFetchNextPageError
  const failedQuery =
    query.isError && !loadMoreFailed
      ? query
      : folderResult.isError
        ? folderResult
        : null
  const loadMoreErrorId = useId()
  const rows = useRef<HTMLTableSectionElement>(null)
  const loadMoreButton = useRef<HTMLButtonElement>(null)
  const focusFrom = useRef<number | null>(null)
  const rowCount = dashboards.length

  const { q, made_by: madeBy, datasource, folder } = search
  const tags = search.tag?.join(",")
  useEffect(() => {
    focusFrom.current = null
  }, [q, madeBy, datasource, tags, folder])

  const fetchingNextPage = query.isFetchingNextPage
  useEffect(() => {
    const from = focusFrom.current
    if (fetchingNextPage || from === null) return
    focusFrom.current = null
    const active = document.activeElement
    const onBody = !active || active === document.body
    const focusLink = (index: number) =>
      rows.current
        ?.querySelector<HTMLElement>(
          `tr[data-dashboard-id="${dashboards[index]?.id}"] a`
        )
        ?.focus()
    if (rowCount > from) {
      if (!onBody && active !== loadMoreButton.current) return
      focusLink(from)
    } else if (onBody) {
      focusLink(rowCount - 1)
    }
  }, [rowCount, fetchingNextPage, dashboards])

  const loadMore = () => {
    focusFrom.current = rowCount
    void query.fetchNextPage()
  }

  const toggleFolder = (folderId: string) =>
    setCollapsed((current) => {
      const next = new Set(current)
      if (!next.delete(folderId)) next.add(folderId)
      return next
    })

  return (
    <>
      {noDashboards ? null : (
        <FilterRow
          key={resetKey}
          search={search}
          onSearchChange={onSearchChange}
          summary={
            query.isPlaceholderData || sections === null
              ? undefined
              : listSummary(
                  query.data?.pages[0].total_count ?? 0,
                  sections.length
                )
          }
        />
      )}
      {failedQuery === null ? null : (
        <div className="mb-4">
          <LoadProblem
            title={
              failedQuery === query
                ? "Could not load dashboards"
                : "Could not load folders"
            }
            detail={loadProblemText(failedQuery.error)}
            retrying={failedQuery.isFetching}
            onRetry={() => void failedQuery.refetch()}
          />
        </div>
      )}
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
                {roleAtLeast(principal.role, "editor") ? (
                  <Link
                    to="/import"
                    className={cn(buttonVariants({ variant: "outline" }))}
                  >
                    Import from Grafana
                  </Link>
                ) : null}
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
          <TableCard minWidth={820} labelledBy={labelledBy}>
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
            <tbody ref={rows} aria-busy={pending || query.isPlaceholderData}>
              {sections === null ? (
                <SkeletonRows table={table} />
              ) : (
                sectionRows(sections, collapsed, toggleFolder, table)
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
