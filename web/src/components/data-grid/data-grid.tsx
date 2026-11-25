import {
  ClientSideRowModelApiModule,
  ClientSideRowModelModule,
  RowAutoHeightModule,
  RowStyleModule,
  type CellMouseOverEvent,
  type ColDef,
  type RowHeightParams,
  type GetRowIdParams,
  type ICellRendererParams,
  type IsFullWidthRowParams,
  type IHeaderParams,
  type ProcessRowParams,
  type RowClassRules,
  type SuppressKeyboardEventParams,
} from "ag-grid-community"
import { AgGridReact } from "ag-grid-react"
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react"
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  type ReactNode,
} from "react"

import { cn } from "@/lib/utils"

import { DetailRenderContext, DetailRow, detailRowEstimate } from "./detail-row"
import { gridLayout } from "./layout"
import { gridLines, type GridLine } from "./lines"
import {
  NarrowRow,
  NarrowRowsContext,
  narrowRowEstimate,
  type NarrowRows,
} from "./narrow-row"
import { gridTheme } from "./theme"
import { useFillHeight } from "./use-fill-height"
import { useNarrow } from "./use-narrow"

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
}

export type DataGridSort = {
  key: string
  direction: "ascending" | "descending"
}

type DataGridDetail<T> = {
  open: ReadonlySet<string>
  render: (row: T) => ReactNode
}

type CellParams<T> = ICellRendererParams<GridLine<T>> & {
  column: DataGridColumn<T>
}

function GridCell<T>({ data, column }: CellParams<T>) {
  if (data === undefined) return null
  return (
    <div
      className={cn(
        "flex h-full min-w-0 items-center leading-normal",
        column.align === "right" && "justify-end text-right"
      )}
    >
      {column.cell(data.row)}
    </div>
  )
}

type HeaderParams = IHeaderParams & {
  align: "left" | "right"
  direction: DataGridSort["direction"] | null
  onSort?: () => void
  hidden: boolean
}

function SortHeader({
  displayName,
  eGridHeader,
  align,
  direction,
  onSort,
  hidden,
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
        className={cn(
          "w-full truncate",
          align === "right" && "text-right",
          hidden && "sr-only"
        )}
      >
        {displayName}
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
      {displayName}
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

function FullWidthRow<T>(params: ICellRendererParams<GridLine<T>>) {
  return params.data?.detail === true ? (
    <DetailRow {...params} />
  ) : (
    <NarrowRow {...params} />
  )
}

function detailHeight<T>({ data }: RowHeightParams<GridLine<T>>) {
  return data?.detail === true ? detailRowEstimate : undefined
}

function narrowHeight<T>({ data }: RowHeightParams<GridLine<T>>) {
  return data?.detail === true ? detailRowEstimate : narrowRowEstimate
}

function everyRowFullWidth() {
  return true
}

function detailRowFullWidth<T>({ rowNode }: IsFullWidthRowParams<GridLine<T>>) {
  return rowNode.data?.detail === true
}

export function DataGrid<T>({
  label,
  rows,
  columns,
  rowId,
  sort = null,
  onSort,
  highlightedRowId = null,
  onHighlightRow,
  detail,
  compact = false,
  rowAttributes,
  bare = false,
  empty,
  className,
}: {
  label: string
  rows: readonly T[]
  columns: readonly DataGridColumn<T>[]
  rowId: (row: T) => string
  sort?: DataGridSort | null
  onSort?: (key: string) => void
  highlightedRowId?: string | null
  onHighlightRow?: (rowId: string | null) => void
  detail?: DataGridDetail<T>
  compact?: boolean
  rowAttributes?: (row: T) => Readonly<Record<string, string>>
  bare?: boolean
  empty?: ReactNode
  className?: string
}) {
  const narrow = useNarrow()
  const containerRef = useRef<HTMLDivElement>(null)
  const rowIdRef = useRef(rowId)
  const highlightRef = useRef(onHighlightRow)
  const sortRef = useRef(onSort)
  const rowAttributesRef = useRef(rowAttributes)
  const labelRef = useRef(label)
  useLayoutEffect(() => {
    labelRef.current = label
    rowIdRef.current = rowId
    highlightRef.current = onHighlightRow
    sortRef.current = onSort
    rowAttributesRef.current = rowAttributes
  })
  const sorts = onSort !== undefined
  const sortKey = sort?.key ?? null
  const sortDirection = sort?.direction ?? null
  const open = detail?.open
  const idOf = useCallback((row: T) => rowIdRef.current(row), [])
  // eslint-disable-next-line react-hooks/refs -- rowId is read through a ref so an inline rowId does not rebuild every line on each render
  const lines = useMemo(() => gridLines(rows, idOf, open), [rows, idOf, open])
  const getRowId = useCallback(
    ({ data }: GetRowIdParams<GridLine<T>>) => data.id,
    []
  )
  const rowClassRules = useMemo<RowClassRules<GridLine<T>>>(
    () => ({
      [highlightClass]: ({ node }) =>
        highlightedRowId !== null && node.id === highlightedRowId,
    }),
    [highlightedRowId]
  )
  const processRowPostCreate = useCallback(
    ({ eRow, node }: ProcessRowParams<GridLine<T>>) => {
      const attributes =
        node.data === undefined || node.data.detail
          ? undefined
          : rowAttributesRef.current?.(node.data.row)
      for (const [name, value] of Object.entries(attributes ?? {})) {
        eRow.setAttribute(name, value)
      }
    },
    []
  )
  const highlight = useCallback(
    (id: string | null) => highlightRef.current?.(id),
    []
  )
  const onCellMouseOver = useCallback(
    (event: CellMouseOverEvent<GridLine<T>>) =>
      highlight(event.node.id ?? null),
    [highlight]
  )
  const narrowRows = useMemo(
    () => ({ columns, onHighlight: highlight }) as NarrowRows<unknown>,
    [columns, highlight]
  )
  const renderDetail = detail?.render as
    ((row: unknown) => ReactNode) | undefined
  const hasDetail = detail !== undefined
  const layout = gridLayout(rows.length)
  const height = useFillHeight(containerRef, layout === "normal")
  const rowHeight = compact ? 32 : 40
  const headerHeight = compact ? 32 : 36

  const columnDefs = useMemo<ColDef<GridLine<T>>[]>(
    () =>
      columns.map((column) => ({
        colId: column.id,
        headerName: column.header,
        sortable: false,
        cellRenderer: GridCell,
        cellRendererParams: { column },
        headerComponent: SortHeader,
        headerComponentParams: {
          align: column.align ?? "left",
          hidden: column.action === true,
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
    containerRef.current
      ?.querySelector("[role=grid]")
      ?.setAttribute("aria-label", labelRef.current)
  }, [])
  useEffect(labelGrid, [label, labelGrid])

  if (rows.length === 0 && empty !== undefined) return <>{empty}</>

  return (
    <div
      ref={containerRef}
      data-layout={narrow ? "narrow" : "wide"}
      data-dom-layout={layout}
      className={cn(
        "w-full text-[13px]",
        bare &&
          "[&_.ag-root-wrapper]:rounded-none! [&_.ag-root-wrapper]:border-0! [&_.ag-styled-root]:[--ag-background-color:transparent]! [&_.ag-styled-root]:[--ag-chrome-background-color:transparent]! [&_.ag-styled-root]:[--ag-header-background-color:transparent]!",
        className
      )}
      style={layout === "normal" ? { height } : undefined}
      onPointerLeave={() => onHighlightRow?.(null)}
    >
      <DetailRenderContext value={renderDetail}>
        <NarrowRowsContext value={narrowRows}>
          <AgGridReact<GridLine<T>>
            key={narrow ? "narrow" : "wide"}
            modules={modules}
            theme={gridTheme}
            domLayout={layout}
            rowData={lines}
            columnDefs={columnDefs}
            getRowId={getRowId}
            rowHeight={rowHeight}
            headerHeight={narrow ? 0 : headerHeight}
            suppressMovableColumns
            isFullWidthRow={
              narrow
                ? everyRowFullWidth
                : hasDetail
                  ? detailRowFullWidth
                  : undefined
            }
            fullWidthCellRenderer={FullWidthRow}
            getRowHeight={
              narrow ? narrowHeight : hasDetail ? detailHeight : undefined
            }
            rowClassRules={rowClassRules}
            processRowPostCreate={processRowPostCreate}
            onCellMouseOver={onCellMouseOver}
            onGridReady={labelGrid}
          />
        </NarrowRowsContext>
      </DetailRenderContext>
    </div>
  )
}
