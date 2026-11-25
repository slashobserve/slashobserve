import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { EllipsisIcon, PlusIcon, SearchIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"

import { AuthorMark } from "@/components/author-mark"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import {
  dataCell,
  skeletonRows,
  type SkeletonRow,
} from "@/components/data-grid/skeleton-cell"
import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { clampQuery } from "@/dashboards/list-filters"
import { foldersQuery, type Folder } from "@/folders/queries"
import { markOf } from "@/lib/author"
import { observationLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

import { DeleteObservationsDialog } from "./delete-dialog"
import { ObservationsPagination } from "./list-pagination"
import {
  filtersApplied,
  pageCountOf,
  pageRange,
  pageSizes,
  sortChoices,
  sortValue,
  type ObservationListView,
  type ObservationStateFilter,
} from "./list-view"
import { NewObservationPanel } from "./new-observation-panel"
import {
  observationsPageQuery,
  useReopenObservationRow,
  type Observation,
} from "./queries"
import { ResolveManyDialog } from "./resolve-many-dialog"
import { plainTitle } from "@/comments/plain-title"

export type { ObservationStateFilter } from "./list-view"

type Props = {
  view: ObservationListView
  creating: boolean
  onChange: (next: ObservationListView, creating: boolean) => void
}

const stateItems: { value: ObservationStateFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "resolved", label: "Resolved" },
  { value: "all", label: "All" },
]

function isStateFilter(value: unknown): value is ObservationStateFilter {
  return stateItems.some((item) => item.value === value)
}

const skeletonCount = 3
const tableLabel = "Observations"
const searchDelayMs = 250
const emptyList: Observation[] = []
const noneSelected: ReadonlySet<string> = new Set()

type Action = "resolve" | "reopen" | "delete"

function mayDo(observation: Observation, action: Action): boolean {
  return observation.allowed_actions.includes(action)
}

function TitleSearch({
  value,
  onSearch,
}: {
  value: string
  onSearch: (q: string) => void
}) {
  const [text, setText] = useState(value)
  const emitted = useRef(value)
  const typed = text.trim()

  useEffect(() => {
    if (value === emitted.current) return
    emitted.current = value
    setText(value)
  }, [value])

  useEffect(() => {
    if (typed === value) return
    const timer = setTimeout(() => {
      emitted.current = typed
      onSearch(typed)
    }, searchDelayMs)
    return () => clearTimeout(timer)
  }, [typed, value, onSearch])

  return (
    <label className="flex h-8 w-[260px] max-w-full items-center gap-2 rounded-control border bg-background px-2.5 focus-within:border-ring focus-within:ring-2 focus-within:ring-ring">
      <SearchIcon
        aria-hidden="true"
        className="size-3.5 flex-none text-muted-foreground"
      />
      <span className="sr-only">Search observations by title</span>
      <input
        type="text"
        value={text}
        autoComplete="off"
        placeholder="Search by title"
        onChange={(event) => setText(clampQuery(event.target.value))}
        className="min-w-0 flex-1 border-0 bg-transparent text-[13px] outline-none placeholder:text-muted-foreground"
      />
    </label>
  )
}

export function ObservationList({ view, creating, onChange }: Props) {
  const newButton = useRef<HTMLButtonElement>(null)
  const list = useQuery(
    observationsPageQuery({
      state: view.state === "all" ? undefined : view.state,
      q: view.q,
      mine: view.mine,
      sort: view.sort,
      order: view.order,
      page: view.page,
      size: view.size,
    })
  )
  const folderResult = useQuery(foldersQuery())
  const items = list.data?.observations ?? emptyList
  const total = list.data?.total ?? 0
  const settled = list.data !== undefined && !list.isPlaceholderData
  const pageCount = pageCountOf(total, view.size)
  const clampTo = settled && view.page > pageCount ? pageCount : null
  const range = pageRange(view.page, view.size, total)
  const folderOf = useMemo(
    () => new Map((folderResult.data ?? []).map((f) => [f.id, f])),
    [folderResult.data]
  )
  const filtered = filtersApplied(view)

  const viewKey = JSON.stringify(view)
  const [selection, setSelection] = useState<{
    key: string
    ids: ReadonlySet<string>
  }>({ key: viewKey, ids: new Set() })
  const selected = selection.key === viewKey ? selection.ids : noneSelected
  const setSelected = useCallback(
    (next: ReadonlySet<string>) => setSelection({ key: viewKey, ids: next }),
    [viewKey]
  )
  const [deleting, setDeleting] = useState<readonly Observation[] | null>(null)
  const [resolving, setResolving] = useState<readonly Observation[] | null>(
    null
  )
  const reopen = useReopenObservationRow()

  const shown = useMemo(() => new Set(items.map((o) => o.id)), [items])
  const rows = useMemo<Row[]>(
    () => items.map((item) => ({ kind: "observation", id: item.id, item })),
    [items]
  )
  const chosen = useMemo(
    () => items.filter((o) => selected.has(o.id)),
    [items, selected]
  )

  useEffect(() => {
    if (clampTo !== null) onChange({ ...view, page: clampTo }, creating)
  }, [clampTo, view, creating, onChange])

  function change(next: Partial<ObservationListView>) {
    onChange({ ...view, page: 1, ...next }, creating)
  }

  const toggle = useCallback(
    (id: string, on: boolean) => {
      const next = new Set(selected)
      if (on) next.add(id)
      else next.delete(id)
      setSelected(next)
    },
    [selected, setSelected]
  )

  function forget(ids: readonly string[]) {
    const next = new Set(selected)
    for (const id of ids) next.delete(id)
    setSelected(next)
  }

  const { mutate: reopenMutate } = reopen
  const reopenRow = useCallback(
    (observation: Observation) => {
      reopenMutate(observation.id, {
        onError: (error) =>
          toast.error(
            error instanceof Error && error.message !== ""
              ? error.message
              : "It could not be reopened. Try again."
          ),
      })
    },
    [reopenMutate]
  )
  const selectPage = useCallback(
    (on: boolean) => setSelected(on ? new Set(shown) : new Set()),
    [setSelected, shown]
  )
  const resolveRow = useCallback(
    (observation: Observation) => setResolving([observation]),
    []
  )
  const deleteRow = useCallback(
    (observation: Observation) => setDeleting([observation]),
    []
  )

  const allShown = items.length > 0 && items.every((o) => selected.has(o.id))
  const someShown = items.some((o) => selected.has(o.id))
  const resolvable = chosen.filter((o) => mayDo(o, "resolve"))
  const deletable = chosen.filter((o) => mayDo(o, "delete"))

  const columns = useMemo(
    () =>
      observationColumns({
        folderOf,
        selected,
        allShown,
        someShown,
        onSelectPage: selectPage,
        onSelect: toggle,
        onResolve: resolveRow,
        onReopen: reopenRow,
        onDelete: deleteRow,
      }),
    [
      folderOf,
      selected,
      allShown,
      someShown,
      selectPage,
      toggle,
      resolveRow,
      reopenRow,
      deleteRow,
    ]
  )

  const newObservation = (
    <Button ref={newButton} onClick={() => onChange(view, true)}>
      <PlusIcon aria-hidden="true" />
      New observation
    </Button>
  )

  const summary =
    list.data === undefined || list.isPlaceholderData
      ? undefined
      : total === 0
        ? undefined
        : `${range.first}-${range.last} of ${total}`

  return (
    <>
      <PageHeader title="Observations" actions={newObservation} />
      <FilterToolbar label="Observation filters">
        <Tabs
          value={view.state}
          onValueChange={(next) => {
            if (isStateFilter(next)) change({ state: next })
          }}
        >
          <TabsList variant="line" aria-label="State">
            {stateItems.map((item) => (
              <TabsTrigger key={item.value} value={item.value}>
                {item.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <TitleSearch value={view.q} onSearch={(q) => change({ q })} />
        <Button
          variant="outline"
          aria-pressed={view.mine}
          className={view.mine ? "border-primary text-primary" : undefined}
          onClick={() => change({ mine: !view.mine })}
        >
          Opened by me
        </Button>
        <Select
          items={sortChoices}
          value={sortValue(view)}
          onValueChange={(next) => {
            const choice = sortChoices.find((c) => c.value === next)
            if (choice !== undefined) {
              change({ sort: choice.sort, order: choice.order })
            }
          }}
        >
          <SelectTrigger size="sm" aria-label="Sort observations">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {sortChoices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span
          role="status"
          className="ml-auto text-xs text-muted-foreground tabular-nums"
        >
          {summary}
        </span>
      </FilterToolbar>
      {chosen.length === 0 ? null : (
        <div
          role="group"
          aria-label="Selected observations"
          className="mb-3 flex flex-wrap items-center gap-2 rounded-card border bg-background px-3.5 py-2 text-[13px]"
        >
          <span className="font-medium tabular-nums">
            {chosen.length} selected
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={resolvable.length === 0}
            onClick={() => setResolving(resolvable)}
          >
            Resolve {resolvable.length}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={deletable.length === 0}
            onClick={() => setDeleting(deletable)}
          >
            <Trash2Icon aria-hidden="true" />
            Delete {deletable.length}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelected(new Set())}
          >
            Clear selection
          </Button>
        </div>
      )}
      {list.isError && list.data === undefined ? (
        <LoadProblem
          title="Could not load observations"
          detail={loadProblemText(list.error)}
          retrying={list.isFetching}
          onRetry={() => void list.refetch()}
        />
      ) : list.data === undefined ? (
        <ObservationSkeleton />
      ) : items.length === 0 && clampTo === null ? (
        <div className="rounded-card border bg-background">
          {filtered ? (
            <EmptyState
              title="No observations match these filters."
              action={
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    onChange(
                      {
                        ...view,
                        state: "open",
                        q: "",
                        mine: false,
                        page: 1,
                      },
                      creating
                    )
                  }
                >
                  Clear filters
                </Button>
              }
            />
          ) : (
            <EmptyState title="No observations yet" action={newObservation}>
              An observation is a page of evidence and notes that people and
              agents build together.
            </EmptyState>
          )}
        </div>
      ) : (
        <>
          {folderResult.isError ? (
            <div className="mb-3">
              <LoadProblem
                title="Could not load folders"
                detail={loadProblemText(folderResult.error)}
                retrying={folderResult.isFetching}
                onRetry={() => void folderResult.refetch()}
              />
            </div>
          ) : null}
          <DataGrid
            label={tableLabel}
            busy={list.isPlaceholderData || clampTo !== null}
            rows={rows}
            columns={columns}
            rowId={rowId}
            selectedRowIds={selected}
          />
          {pageCount > 1 || total > pageSizes[0] ? (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                Per page
                <Select
                  items={pageSizes.map((n) => ({
                    value: String(n),
                    label: String(n),
                  }))}
                  value={String(view.size)}
                  onValueChange={(next) => {
                    const size = pageSizes.find((n) => String(n) === next)
                    if (size !== undefined) change({ size })
                  }}
                >
                  <SelectTrigger size="sm" aria-label="Observations per page">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false} align="start">
                    {pageSizes.map((n) => (
                      <SelectItem key={n} value={String(n)}>
                        {n}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </span>
              {pageCount > 1 ? (
                <ObservationsPagination view={view} pageCount={pageCount} />
              ) : null}
            </div>
          ) : null}
        </>
      )}
      {creating ? (
        <NewObservationPanel
          trigger={newButton}
          onClose={() => onChange(view, false)}
        />
      ) : null}
      <DeleteObservationsDialog
        targets={deleting ?? []}
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        onDeleted={forget}
      />
      <ResolveManyDialog
        targets={resolving ?? []}
        open={resolving !== null}
        onOpenChange={(open) => {
          if (!open) setResolving(null)
        }}
        onResolved={forget}
      />
    </>
  )
}

type Row = { kind: "observation"; id: string; item: Observation } | SkeletonRow

type ColumnProps = {
  folderOf: ReadonlyMap<string, Folder>
  selected: ReadonlySet<string>
  allShown: boolean
  someShown: boolean
  onSelectPage: (on: boolean) => void
  onSelect: (id: string, on: boolean) => void
  onResolve: (observation: Observation) => void
  onReopen: (observation: Observation) => void
  onDelete: (observation: Observation) => void
}

const quiet = "motion-safe:animate-none"

function observationColumns({
  folderOf,
  selected,
  allShown,
  someShown,
  onSelectPage,
  onSelect,
  onResolve,
  onReopen,
  onDelete,
}: ColumnProps): DataGridColumn<Row>[] {
  return [
    {
      id: "title",
      header: "Title",
      headerCell: () => (
        <span className="inline-flex items-center gap-2.5">
          <Checkbox
            aria-label="Select every observation on this page"
            checked={allShown}
            indeterminate={someShown && !allShown}
            onCheckedChange={onSelectPage}
          />
          Title
        </span>
      ),
      minWidth: 260,
      flex: 3,
      autoHeight: true,
      cell: dataCell<Row, "observation">(
        "observation",
        `h-4 w-56 ${quiet}`,
        ({ item }) => (
          <span className="inline-flex items-center gap-2.5 py-2 font-medium wrap-anywhere">
            <Checkbox
              aria-label={`Select ${plainTitle(item.title)}`}
              checked={selected.has(item.id)}
              onCheckedChange={(on) => onSelect(item.id, on)}
            />
            <Link
              {...observationLink(item.id)}
              className="underline-offset-4 hover:underline"
            >
              {plainTitle(item.title)}
            </Link>
          </span>
        )
      ),
    },
    {
      id: "folder",
      header: "Folder",
      minWidth: 120,
      flex: 1,
      autoHeight: true,
      cell: dataCell<Row, "observation">(
        "observation",
        `h-4 w-24 ${quiet}`,
        ({ item }) => {
          const folder = folderOf.get(item.folder_id)
          return folder === undefined ? null : (
            <Link
              to="/f/$folderId"
              params={{ folderId: folder.id }}
              className="wrap-anywhere underline-offset-4 hover:underline"
            >
              {folder.title}
            </Link>
          )
        }
      ),
    },
    {
      id: "state",
      header: "State",
      width: 110,
      cell: dataCell<Row, "observation">(
        "observation",
        `h-4 w-14 ${quiet}`,
        ({ item }) =>
          item.state === "open" ? (
            <StatusLabel tone="ok">Open</StatusLabel>
          ) : (
            <StatusLabel tone="none">Resolved</StatusLabel>
          )
      ),
    },
    {
      id: "openedBy",
      header: "Opened by",
      minWidth: 150,
      flex: 1,
      cell: dataCell<Row, "observation">(
        "observation",
        `h-4 w-28 ${quiet}`,
        ({ item }) => (
          <span className="inline-flex max-w-full items-center gap-2">
            <AuthorMark mark={markOf(item.created_by)} size={20} />
            <span className="truncate" title={item.created_by.label}>
              {item.created_by.label}
            </span>
          </span>
        )
      ),
    },
    {
      id: "updated",
      header: "Updated",
      width: 110,
      cell: dataCell<Row, "observation">(
        "observation",
        `h-4 w-12 ${quiet}`,
        ({ item }) => (
          <span className="whitespace-nowrap text-muted-foreground">
            <RelativeTime iso={item.updated_at} />
          </span>
        )
      ),
    },
    {
      id: "actions",
      header: "Actions",
      headerCell: () => <span className="sr-only">Actions</span>,
      width: 56,
      action: true,
      cell: dataCell<Row, "observation">("observation", "", ({ item }) => {
        const canResolve = mayDo(item, "resolve")
        const canReopen = mayDo(item, "reopen")
        const canDelete = mayDo(item, "delete")
        if (!canResolve && !canReopen && !canDelete) return null
        return (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Actions for ${plainTitle(item.title)}`}
                />
              }
            >
              <EllipsisIcon aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-max">
              {canResolve ? (
                <DropdownMenuItem onClick={() => onResolve(item)}>
                  Resolve
                </DropdownMenuItem>
              ) : null}
              {canReopen ? (
                <DropdownMenuItem onClick={() => onReopen(item)}>
                  Reopen
                </DropdownMenuItem>
              ) : null}
              {canDelete ? (
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => onDelete(item)}
                >
                  <Trash2Icon aria-hidden="true" />
                  Delete
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        )
      }),
    },
  ]
}

const rowId = (row: Row) => row.id
const skeleton = skeletonRows(skeletonCount)
const skeletonColumns = observationColumns({
  folderOf: new Map(),
  selected: noneSelected,
  allShown: false,
  someShown: false,
  onSelectPage: () => undefined,
  onSelect: () => undefined,
  onResolve: () => undefined,
  onReopen: () => undefined,
  onDelete: () => undefined,
}).map((column) =>
  column.id === "title" ? { ...column, headerCell: undefined } : column
)

function ObservationSkeleton() {
  return (
    <DataGrid
      label={tableLabel}
      busy
      rows={skeleton}
      columns={skeletonColumns}
      rowId={rowId}
    />
  )
}
