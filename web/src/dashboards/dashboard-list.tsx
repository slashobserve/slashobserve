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
import { moveChoices } from "@/folders/move-targets"
import { foldersQuery } from "@/folders/queries"
import { importedFilter } from "@/health/health-labels"
import { ListHealthProvider, useListHealthLoad } from "@/health/list-health"
import { RepairQueue } from "@/import/repair/repair-page"
import { roleAtLeast } from "@/lib/roles"
import { docsPageUrl } from "@/lib/docs"
import { cn } from "@/lib/utils"
import { levelLabels } from "@/folders/grants"
import { loadProblemText } from "@/service-accounts/queries"

import {
  actionsColumn,
  dashboardColumns,
  folderColumn,
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
  foldersToBrowse,
  hasFilters,
  leavesOutAudit,
  pageSearch,
  toFilters,
  withoutPage,
  type ListSearch,
} from "./list-filters"
import { BulkMoveBar } from "./bulk-move-bar"
import { useDashboardListLive } from "./live/use-dashboards-live"
import { ListSelectionProvider, type ListSelection } from "./list-selection"
import { ListPagination } from "./list-pagination"
import {
  listOffset,
  listPageSize,
  pageCountOf,
  pageOutOfRange,
  pageRange,
  withOffset,
} from "./list-pages"
import { SortMenu } from "./sort-menu"

const skeletonRows = 5
const searchDelayMs = 250
const emptyList: DashboardSummary[] = []
const groupedColumns = [...dashboardColumns, actionsColumn]
const flatColumns = [
  dashboardColumns[0],
  folderColumn,
  ...dashboardColumns.slice(1),
  actionsColumn,
]
const dashboardRowId = (dashboard: DashboardSummary) => dashboard.id

function useDashboardList(
  search: ListSearch,
  page: number,
  excludeAudit: boolean
) {
  const filters = { ...toFilters(search), excludeAudit }
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
              exclude_audit: filters.excludeAudit ? true : undefined,
              order: filters.order ?? "folder",
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

function useListTable(data: DashboardSummary[], flat: boolean) {
  return useTable({
    features: listFeatures,
    columns: flat ? flatColumns : groupedColumns,
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
  const hideAudit = leavesOutAudit(search, folderLocked)
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
        excludeAudit={hideAudit}
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
        excludeAudit={hideAudit}
        value={search.tag}
        onChange={(tag) => onSearchChange({ ...search, tag })}
      />
      {search.kind === importedFilter ? null : (
        <SortMenu
          value={search.sort}
          onChange={(sort) => onSearchChange({ ...search, sort })}
        />
      )}
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

function flatRows(table: ListTable): ReactNode[] {
  return table.getRowModel().rows.map((row) => (
    <tr key={row.id} data-dashboard-id={row.id} className="group/row">
      {row.getAllCells().map((cell) => (
        <Cell
          key={cell.id}
          data-cell={cell.column.id === actionsColumn.id ? "action" : undefined}
          data-label={cell.column.columnDef.meta?.stackedLabel}
          className={cell.column.columnDef.meta?.cellClassName}
        >
          <table.FlexRender cell={cell} />
        </Cell>
      ))}
    </tr>
  ))
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
                data-label={cell.column.columnDef.meta?.stackedLabel}
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
  useDashboardListLive()
  const folderLocked = folderId !== undefined
  const page = search.page ?? 1
  const hideAudit = leavesOutAudit(search, folderLocked)
  const query = useDashboardList(
    folderLocked ? { ...search, folder: folderId } : search,
    page,
    hideAudit
  )
  const folderResult = useQuery(foldersQuery())
  const dashboards = query.data?.dashboards ?? emptyList
  const healthIds = useMemo(
    () => dashboards.map((dashboard) => dashboard.id),
    [dashboards]
  )
  const total = query.data?.total_count ?? 0
  const dataOffset = query.data?.offset ?? 0
  const flat = search.sort !== undefined
  const table = useListTable(dashboards, flat)
  const allFolders = folderResult.data
  const folders = useMemo(
    () =>
      folderLocked
        ? allFolders?.filter((folder) => folder.id === folderId)
        : allFolders,
    [allFolders, folderLocked, folderId]
  )
  const listedFolders = useMemo(
    () =>
      hideAudit && folders !== undefined
        ? foldersToBrowse(folders, undefined)
        : folders,
    [hideAudit, folders]
  )
  const readOnlyFolder = folders?.find(
    (folder) => folderLocked && !folder.permissions.edit
  )
  const filtered = hasFilters(search)
  const loaded = query.data !== undefined
  const settled = loaded && !query.isPlaceholderData
  const clampTo = settled ? pageOutOfRange(page, total) : null
  const sections = useMemo(
    () =>
      !flat && loaded && listedFolders !== undefined && clampTo === null
        ? groupPage(listedFolders, dashboards, {
            offset: dataOffset,
            total,
            filtered,
          })
        : null,
    [
      flat,
      loaded,
      listedFolders,
      clampTo,
      dashboards,
      dataOffset,
      total,
      filtered,
    ]
  )
  const ready = flat
    ? loaded && listedFolders !== undefined && clampTo === null
    : sections !== null
  const rowCount = flat ? dashboards.length : (sections?.length ?? 0)
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const selectionScope = JSON.stringify([search, page, folderId])
  const [chosen, setChosen] = useState<{
    scope: string
    ids: ReadonlySet<string>
  }>({ scope: selectionScope, ids: new Set() })
  const canMoveInBulk =
    folders !== undefined && moveChoices(folders, null).length > 0
  const selectable = useMemo(
    () =>
      canMoveInBulk && !query.isPlaceholderData
        ? dashboards
            .filter((dashboard) => dashboard.permissions.edit)
            .map((dashboard) => dashboard.id)
        : [],
    [canMoveInBulk, query.isPlaceholderData, dashboards]
  )
  const selected = useMemo(
    () =>
      chosen.scope === selectionScope
        ? new Set(selectable.filter((id) => chosen.ids.has(id)))
        : new Set<string>(),
    [chosen, selectionScope, selectable]
  )
  const selection: ListSelection | null =
    selectable.length === 0
      ? null
      : {
          selectable,
          selected,
          toggle: (id) => {
            const ids = new Set(selected)
            if (!ids.delete(id)) ids.add(id)
            setChosen({ scope: selectionScope, ids })
          },
          togglePage: () =>
            setChosen({
              scope: selectionScope,
              ids:
                selected.size === selectable.length
                  ? new Set()
                  : new Set(selectable),
            }),
        }
  const selectedDashboards = dashboards.filter((dashboard) =>
    selected.has(dashboard.id)
  )
  const noDashboards =
    listedFolders !== undefined &&
    loaded &&
    dashboards.length === 0 &&
    !query.isPlaceholderData &&
    !filtered &&
    clampTo === null &&
    (folderLocked || !hasPeopleFolders(listedFolders))
  const empty = ready && (noDashboards || rowCount === 0)
  const [resetKey, setResetKey] = useState(0)
  const pending = query.isPending || folderResult.isPending
  const showTable = pending || clampTo !== null || (ready && !empty)
  const failedQuery = query.isError
    ? query
    : folderResult.isError
      ? folderResult
      : null
  const regionRef = useRef<HTMLDivElement>(null)
  const retryRef = useRef<HTMLButtonElement>(null)
  const focusIntent = useRef<number | null>(null)

  const { q, made_by: madeBy, datasource, folder, kind, sort } = search
  const tags = search.tag?.join(",")
  useEffect(() => {
    focusIntent.current = null
  }, [q, madeBy, datasource, tags, folder, kind, sort])

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
    : query.isPlaceholderData || !ready
      ? undefined
      : (folderLocked || flat) && pageCount === 1
        ? dashboardCountText(total)
        : listSummary(total, sections?.length ?? 0, page)

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
      <>
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
            {noDashboards && readOnlyFolder !== undefined ? (
              <>
                <h2 className="text-lg font-semibold">No dashboards yet</h2>
                <p className="text-sm text-muted-foreground">
                  {`Your access: ${levelLabels[readOnlyFolder.level]}. Someone with Edit on ${readOnlyFolder.title} adds dashboards here.`}
                </p>
              </>
            ) : noDashboards ? (
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
                <a
                  href={docsPageUrl("get-started/first-dashboard")}
                  target="_blank"
                  rel="noreferrer"
                  className="text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
                >
                  Read about this in the docs
                  <span className="sr-only"> (opens in a new tab)</span>
                </a>
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
        {selectedDashboards.length > 0 && folders !== undefined ? (
          <BulkMoveBar
            selected={selectedDashboards}
            folders={folders}
            onClear={() => setChosen({ scope: selectionScope, ids: new Set() })}
            onMoved={(moved) => {
              const left = new Set(selected)
              for (const id of moved) left.delete(id)
              setChosen({ scope: selectionScope, ids: left })
              regionRef.current?.focus({ preventScroll: true })
            }}
          />
        ) : null}
        {showTable ? (
          <ListSelectionProvider value={selection}>
            <TableCard
              minWidth={700}
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
                        className={
                          header.column.columnDef.meta?.headerClassName
                        }
                      >
                        <table.FlexRender header={header} />
                      </HeaderCell>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody
                aria-busy={
                  pending || query.isPlaceholderData || clampTo !== null
                }
              >
                {!ready ? (
                  <SkeletonRows table={table} />
                ) : sections === null ? (
                  flatRows(table)
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
          </ListSelectionProvider>
        ) : null}
      </>
    </ListHealthProvider>
  )
}
