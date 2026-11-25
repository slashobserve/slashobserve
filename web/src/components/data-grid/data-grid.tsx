import {
  ClientSideRowModelApiModule,
  ClientSideRowModelModule,
  RowAutoHeightModule,
  RowStyleModule,
  type CellMouseOverEvent,
  type ColDef,
  type GetRowIdParams,
  type ICellRendererParams,
  type IHeaderParams,
  type RowClassRules,
  type RowHeightParams,
  type SuppressKeyboardEventParams,
} from "ag-grid-community"
import { AgGridReact } from "ag-grid-react"
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react"
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useImperativeHandle,
  useRef,
  type ReactNode,
  type Ref,
} from "react"

import { cn } from "@/lib/utils"

import { gridLayout } from "./layout"
import {
  NarrowRow,
  NarrowRowsContext,
  narrowRowEstimate,
  type FullWidthRow,
  type NarrowRows,
} from "./narrow-row"
import { gridTheme } from "./theme"
import { useFillHeight } from "./use-fill-height"
import { useNarrow } from "./use-narrow"
import { useWidth } from "./use-width"

const modules = [
  ClientSideRowModelModule,
  RowAutoHeightModule,
  RowStyleModule,
  ClientSideRowModelApiModule,
]

export type DataGridColumn<T> = {
  id: string
  header: string
  cell: (row: T) => ReactNode
  align?: "left" | "right"
  width?: number
  minWidth?: number
  flex?: number
  sortable?: boolean
  autoHeight?: boolean
  action?: boolean
  headerCell?: () => ReactNode
  hideBelow?: number
}

export type DataGridSort = {
  key: string
  direction: "ascending" | "descending"
}

type CellParams<T> = ICellRendererParams<T> & { column: DataGridColumn<T> }

function GridCell<T>({ data, column }: CellParams<T>) {
  if (data === undefined) return null
  return (
    <div
      className={cn(
        "flex h-full min-w-0 items-center leading-normal",
        column.align === "right" && "justify-end text-right"
      )}
    >
      {column.cell(data)}
    </div>
  )
}

type HeaderParams = IHeaderParams & {
  headerCell?: () => ReactNode
  align: "left" | "right"
  direction: DataGridSort["direction"] | null
  onSort?: () => void
}

function SortHeader({
  displayName,
  headerCell,
  eGridHeader,
  align,
  direction,
  onSort,
}: HeaderParams) {
  const sortRef = useRef(onSort)
  useLayoutEffect(() => {
    sortRef.current = onSort
    if (onSort === undefined) eGridHeader.removeAttribute("aria-sort")
    else eGridHeader.setAttribute("aria-sort", direction ?? "none")
  })
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.target !== eGridHeader) return
      if (event.key !== "Enter" && event.key !== " ") return
      if (sortRef.current === undefined) return
      event.preventDefault()
      sortRef.current()
    }
    eGridHeader.addEventListener("keydown", onKeyDown)
    return () => eGridHeader.removeEventListener("keydown", onKeyDown)
  }, [eGridHeader])

  if (onSort === undefined) {
    return (
      <span
        className={cn("w-full truncate", align === "right" && "text-right")}
      >
        {headerCell?.() ?? displayName}
      </span>
    )
  }
  const Arrow = direction === "ascending" ? ChevronUpIcon : ChevronDownIcon
  return (
    <button
      type="button"
      tabIndex={-1}
      onClick={onSort}
      className={cn(
        "flex h-full w-full items-center gap-1 text-left hover:text-foreground",
        align === "right" && "flex-row-reverse text-right",
        direction !== null && "text-foreground"
      )}
    >
      {headerCell?.() ?? displayName}
      {direction !== null ? (
        <Arrow className="size-3" aria-hidden="true" />
      ) : null}
    </button>
  )
}

function activateCellControl({ event }: SuppressKeyboardEventParams): boolean {
  if (event.type !== "keydown" || event.key !== "Enter") return false
  const cell = event.target
  if (!(cell instanceof HTMLElement)) return false
  const control = cell.querySelector<HTMLElement>("a[href], button")
  if (control === null) return false
  event.preventDefault()
  control.click()
  return true
}

const highlightClass = "bg-selection!"

const everyRowFullWidth = () => true

export function DataGrid<T>({
  label,
  labelledBy,
  rows,
  columns: allColumns,
  rowId,
  sort = null,
  onSort,
  highlightedRowId = null,
  onHighlightRow,
  compact = false,
  empty,
  fullWidthRow,
  busy = false,
  ref,
  className,
}: {
  label?: string
  labelledBy?: string
  rows: readonly T[]
  columns: readonly DataGridColumn<T>[]
  rowId: (row: T) => string
  sort?: DataGridSort | null
  onSort?: (key: string) => void
  highlightedRowId?: string | null
  onHighlightRow?: (rowId: string | null) => void
  compact?: boolean
  empty?: ReactNode
  fullWidthRow?: FullWidthRow<T>
  busy?: boolean
  ref?: Ref<HTMLDivElement>
  className?: string
}) {
  const narrow = useNarrow()
  const containerRef = useRef<HTMLDivElement>(null)
  const hides = allColumns.some((column) => column.hideBelow !== undefined)
  const width = useWidth(containerRef, hides && !narrow)
  const columns = useMemo(
    () =>
      width === 0
        ? allColumns
        : allColumns.filter(
            (column) =>
              column.hideBelow === undefined || width >= column.hideBelow
          ),
    [allColumns, width]
  )
  const rowIdRef = useRef(rowId)
  const highlightRef = useRef(onHighlightRow)
  const sortRef = useRef(onSort)
  const nameRef = useRef({ label, labelledBy })
  useImperativeHandle(ref, () => containerRef.current as HTMLDivElement)
  useLayoutEffect(() => {
    nameRef.current = { label, labelledBy }
    rowIdRef.current = rowId
    highlightRef.current = onHighlightRow
    sortRef.current = onSort
  })
  const sorts = onSort !== undefined
  const sortKey = sort?.key ?? null
  const sortDirection = sort?.direction ?? null
  const getRowId = useCallback(
    ({ data }: GetRowIdParams<T>) => rowIdRef.current(data),
    []
  )
  const rowClassRules = useMemo<RowClassRules<T>>(
    () => ({
      [highlightClass]: ({ node }) =>
        highlightedRowId !== null && node.id === highlightedRowId,
    }),
    [highlightedRowId]
  )
  const highlight = useCallback(
    (id: string | null) => highlightRef.current?.(id),
    []
  )
  const onCellMouseOver = useCallback(
    (event: CellMouseOverEvent<T>) => highlight(event.node.id ?? null),
    [highlight]
  )
  const narrowRows = useMemo(
    () =>
      ({
        columns,
        onHighlight: highlight,
        fullWidth: fullWidthRow,
      }) as NarrowRows<unknown>,
    [columns, highlight, fullWidthRow]
  )
  const isFullWidthRow = useMemo(
    () =>
      narrow
        ? everyRowFullWidth
        : fullWidthRow === undefined
          ? undefined
          : ({ rowNode }: { rowNode: { data: T | undefined } }) =>
              rowNode.data !== undefined && fullWidthRow.is(rowNode.data),
    [narrow, fullWidthRow]
  )
  const getRowHeight = useCallback(
    ({ data }: RowHeightParams<T>) =>
      narrow || (data !== undefined && fullWidthRow?.is(data))
        ? narrowRowEstimate
        : undefined,
    [narrow, fullWidthRow]
  )
  const layout = gridLayout(rows.length)
  const height = useFillHeight(containerRef, layout === "normal")
  const rowHeight = compact ? 32 : 40
  const headerHeight = compact ? 32 : 36

  const columnDefs = useMemo<ColDef<T>[]>(
    () =>
      columns.map((column) => ({
        colId: column.id,
        headerName: column.header,
        sortable: false,
        cellRenderer: GridCell,
        cellRendererParams: { column },
        headerComponent: SortHeader,
        headerComponentParams: {
          headerCell: column.headerCell,
          align: column.align ?? "left",
          direction: sortKey === column.id ? sortDirection : null,
          onSort:
            column.sortable && sorts
              ? () => sortRef.current?.(column.id)
              : undefined,
        },
        ...(narrow
          ? { flex: 1 }
          : {
              width: column.width,
              minWidth: column.minWidth,
              flex: column.width === undefined ? (column.flex ?? 1) : undefined,
              autoHeight: column.autoHeight,
            }),
        suppressKeyboardEvent: activateCellControl,
      })),
    [columns, narrow, sorts, sortKey, sortDirection]
  )

  const labelGrid = useCallback(() => {
    const grid = containerRef.current?.querySelector("[role=grid]")
    if (grid === null || grid === undefined) return
    const name = nameRef.current
    for (const [attribute, value] of [
      ["aria-label", name.label],
      ["aria-labelledby", name.labelledBy],
    ] as const) {
      if (value === undefined) grid.removeAttribute(attribute)
      else grid.setAttribute(attribute, value)
    }
  }, [])
  useEffect(labelGrid, [label, labelledBy, labelGrid])

  if (rows.length === 0 && empty !== undefined) return <>{empty}</>

  return (
    <div
      ref={containerRef}
      data-layout={narrow ? "narrow" : "wide"}
      data-dom-layout={layout}
      tabIndex={-1}
      aria-busy={busy || undefined}
      className={cn("w-full text-[13px] outline-none", className)}
      style={layout === "normal" ? { height } : undefined}
      onPointerLeave={() => onHighlightRow?.(null)}
    >
      <NarrowRowsContext value={narrowRows}>
        <AgGridReact<T>
          key={narrow ? "narrow" : "wide"}
          modules={modules}
          theme={gridTheme}
          domLayout={layout}
          rowData={rows as T[]}
          columnDefs={columnDefs}
          getRowId={getRowId}
          rowHeight={rowHeight}
          headerHeight={narrow ? 0 : headerHeight}
          suppressMovableColumns
          isFullWidthRow={isFullWidthRow}
          fullWidthCellRenderer={NarrowRow}
          getRowHeight={
            narrow || fullWidthRow !== undefined ? getRowHeight : undefined
          }
          rowClassRules={rowClassRules}
          onCellMouseOver={onCellMouseOver}
          onGridReady={labelGrid}
        />
      </NarrowRowsContext>
    </div>
  )
}
