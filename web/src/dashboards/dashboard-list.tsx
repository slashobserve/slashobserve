import { useQuery } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { useTable } from "@tanstack/react-table"
import { SearchIcon } from "lucide-react"
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { RoleGate } from "@/components/role-gate"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { FolderHeaderRow } from "@/folders/folder-header-row"
import { foldersQuery } from "@/folders/queries"
import { importedFilter } from "@/health/health-labels"
import { ListHealthProvider, useListHealthLoad } from "@/health/list-health"
import { RepairQueue } from "@/import/repair/repair-page"
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
  dashboardCountText,
  groupPage,
  hasPeopleFolders,
  listSummary,
  type FolderSection,
} from "./folder-groups"
import {
  DatasourceMenu,
  FolderMenu,
  MadeByMenu,
  ProblemMenu,
  TagsMenu,
} from "./list-filter-menus"
import {
  clampQuery,
  hasFilters,
  pageSearch,
  toFilters,
  withoutPage,
  type ListSearch,
} from "./list-filters"
import { ListPagination } from "./list-pagination"
import {
  listOffset,
  listPageSize,
  pageCountOf,
  pageOutOfRange,
  pageRange,
  withOffset,
} from "./list-pages"

const skeletonRows = 5
const searchDelayMs = 250
const emptyList: DashboardSummary[] = []
const listColumns = [...dashboardColumns, actionsColumn]
const dashboardRowId = (dashboard: DashboardSummary) => dashboard.id

function useDashboardList(search: ListSearch, page: number) {
  const filters = toFilters(search)
  const offset = listOffset(page)
  return useQuery({
    queryKey: dashboardKeys.filtered(filters, page),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: {
            query: {
              limit: listPageSize,
              offset,
              q: filters.q,
              made_by: filters.madeBy,
              datasource: filters.datasource,
              tag: filters.tags,
              folder: filters.folder,
              problem: filters.problem,
              order: "folder",
            },
          },
          signal,
        })
        .then(unwrap)
        .then((list) => withOffset(list, offset)),
    enabled: search.kind !== importedFilter,
    staleTime: 15_000,
    placeholderData: (previous) => previous,
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
  folderLocked: boolean
  offerImported: boolean
}

function FilterRow({
  search,
  onSearchChange,
  summary,
  folderLocked,
  offerImported,
}: FilterRowProps) {
  const health = useListHealthLoad()
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
      {folderLocked ? null : (
        <FolderMenu
          value={search.folder}
          onChange={(folder) => onSearchChange({ ...search, folder })}
        />
      )}
      <ProblemMenu
        value={search.kind}
        offerImported={offerImported}
        onChange={(kind) => onSearchChange({ ...search, kind })}
      />
      <TagsMenu
        value={search.tag}
        onChange={(tag) => onSearchChange({ ...search, tag })}
      />
      <span className="ml-auto flex items-center gap-x-2 text-xs text-muted-foreground">
        <span role="status" className="flex items-center gap-x-2">
          {summary}
          {health.failed ? (
            <StatusLabel tone="warning">Health could not load</StatusLabel>
          ) : null}
        </span>
        {health.failed ? (
          <Button
            variant="ghost"
            size="xs"
            disabled={health.retrying}
            aria-busy={health.retrying}
            onClick={health.retry}
          >
            Retry
          </Button>
        ) : null}
      </span>
    </div>
  )
}

function sectionRows(
  sections: FolderSection[],
  collapsed: ReadonlySet<string>,
  toggle: (folderId: string) => void,
  table: ListTable,
  folderLocked: boolean
): ReactNode[] {
  const columnCount = table.getAllLeafColumns().length
  const tableRows = new Map(
    table.getRowModel().rows.map((row) => [row.id, row])
  )
  return sections.flatMap((section) => {
    const isCollapsed = !folderLocked && collapsed.has(section.folder.id)
    const header = folderLocked ? null : (
      <FolderHeaderRow
        key={`folder:${section.folder.id}`}
        folder={section.folder}
        collapsed={isCollapsed}
        continued={section.continued}
        countText={
          section.count === null ? null : dashboardCountText(section.count)
        }
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
                data-cell={
                  cell.column.id === actionsColumn.id ? "action" : undefined
                }
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
  folderId?: string
}

export function DashboardList({
  labelledBy,
  search,
  onSearchChange,
  folderId,
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const folderLocked = folderId !== undefined
  const page = search.page ?? 1
  const query = useDashboardList(
    folderLocked ? { ...search, folder: folderId } : search,
    page
  )
  const folderResult = useQuery(foldersQuery())
  const dashboards = query.data?.dashboards ?? emptyList
  const healthIds = useMemo(
    () => dashboards.map((dashboard) => dashboard.id),
    [dashboards]
  )
  const total = query.data?.total_count ?? 0
  const dataOffset = query.data?.offset ?? 0
  const table = useListTable(dashboards)
  const allFolders = folderResult.data
  const folders = useMemo(
    () =>
      folderLocked
        ? allFolders?.filter((folder) => folder.id === folderId)
        : allFolders,
    [allFolders, folderLocked, folderId]
  )
  const filtered = hasFilters(search)
  const loaded = query.data !== undefined
  const settled = loaded && !query.isPlaceholderData
  const clampTo = settled ? pageOutOfRange(page, total) : null
  const sections = useMemo(
    () =>
      loaded && folders !== undefined && clampTo === null
        ? groupPage(folders, dashboards, {
            offset: dataOffset,
            total,
            filtered,
          })
        : null,
    [loaded, folders, clampTo, dashboards, dataOffset, total, filtered]
  )
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const noDashboards =
    folders !== undefined &&
    loaded &&
    dashboards.length === 0 &&
    !query.isPlaceholderData &&
    !filtered &&
    clampTo === null &&
    (folderLocked || !hasPeopleFolders(folders))
  const empty = sections !== null && (noDashboards || sections.length === 0)
  const [resetKey, setResetKey] = useState(0)
  const pending = query.isPending || folderResult.isPending
  const showTable = pending || clampTo !== null || (sections !== null && !empty)
  const failedQuery = query.isError
    ? query
    : folderResult.isError
      ? folderResult
      : null
  const regionRef = useRef<HTMLDivElement>(null)
  const retryRef = useRef<HTMLButtonElement>(null)
  const focusIntent = useRef<number | null>(null)

  const { q, made_by: madeBy, datasource, folder, kind } = search
  const tags = search.tag?.join(",")
  useEffect(() => {
    focusIntent.current = null
  }, [q, madeBy, datasource, tags, folder, kind])

  useEffect(() => {
    if (clampTo === null) return
    focusIntent.current = null
    onSearchChange(pageSearch(search, clampTo))
  }, [clampTo, search, onSearchChange])

  const failed = query.isError
  useEffect(() => {
    if (focusIntent.current !== page) return
    if (failed) {
      focusIntent.current = null
      retryRef.current?.focus()
    } else if (settled && clampTo === null) {
      focusIntent.current = null
      regionRef.current?.focus({ preventScroll: true })
      regionRef.current?.scrollIntoView({ block: "start" })
    }
  }, [page, failed, settled, clampTo])

  const onNavigate = (target: number) => {
    focusIntent.current = target
  }
  const onFilterChange = useCallback(
    (next: ListSearch) => onSearchChange(withoutPage(next)),
    [onSearchChange]
  )

  const toggleFolder = (folderId: string) =>
    setCollapsed((current) => {
      const next = new Set(current)
      if (!next.delete(folderId)) next.add(folderId)
      return next
    })

  const pageCount = pageCountOf(total)
  const range = pageRange(page, total)
  const summary = query.isPaused
    ? "Offline. The list loads when you are back online."
    : query.isPlaceholderData || sections === null
      ? undefined
      : folderLocked && pageCount === 1
        ? dashboardCountText(total)
        : listSummary(total, sections.length, page)

  const offerImported =
    roleAtLeast(principal.role, "editor") || kind === importedFilter
  const kindOnly =
    kind !== undefined && !hasFilters({ ...search, kind: undefined })

  if (kind === importedFilter) {
    return (
      <>
        <FilterRow
          key={resetKey}
          search={search}
          onSearchChange={onFilterChange}
          summary={undefined}
          folderLocked={folderLocked}
          offerImported={offerImported}
        />
        <RoleGate
          min="editor"
          note="Only Editors can see the imported dashboards that are not verified."
        >
          <RepairQueue folderId={folderId ?? search.folder ?? null} />
        </RoleGate>
      </>
    )
  }

  return (
    <ListHealthProvider ids={healthIds}>
      {noDashboards ? null : (
        <FilterRow
          key={resetKey}
          search={search}
          onSearchChange={onFilterChange}
          summary={summary}
          folderLocked={folderLocked}
          offerImported={offerImported}
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
            retryRef={retryRef}
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
                  to="/ask"
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
                {kindOnly
                  ? kind === "any"
                    ? "Every dashboard passed its last check."
                    : "No dashboard has this problem."
                  : "No dashboards match these filters."}
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
          <TableCard
            minWidth={960}
            labelledBy={labelledBy}
            ref={regionRef}
            stackOnNarrow
          >
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
              aria-busy={pending || query.isPlaceholderData || clampTo !== null}
            >
              {sections === null ? (
                <SkeletonRows table={table} />
              ) : (
                sectionRows(
                  sections,
                  collapsed,
                  toggleFolder,
                  table,
                  folderLocked
                )
              )}
            </tbody>
          </TableCard>
          {loaded && pageCount > 1 && clampTo === null ? (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
              <p className="text-xs text-muted-foreground tabular-nums">
                {query.isPlaceholderData
                  ? null
                  : `${range.first}-${range.last} of ${total}`}
              </p>
              <ListPagination
                page={page}
                pageCount={pageCount}
                search={search}
                onNavigate={onNavigate}
              />
            </div>
          ) : null}
        </>
      ) : null}
    </ListHealthProvider>
  )
}
