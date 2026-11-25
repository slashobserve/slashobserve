import {
  CellStyleModule,
  ClientSideRowModelApiModule,
  ClientSideRowModelModule,
  ColumnApiModule,
  ColumnAutoSizeModule,
  PinnedRowModule,
  RowApiModule,
  RowAutoHeightModule,
  ScrollApiModule,
  type ColDef,
  type GetRowIdParams,
  type GridApi,
  type GridSizeChangedEvent,
  type ICellRendererParams,
  type IHeaderParams,
  type IsFullWidthRowParams,
  type RowHeightParams,
} from "ag-grid-community"
import { AgGridReact } from "ag-grid-react"
import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  type ReactNode,
  type Ref,
} from "react"

import { cn } from "@/lib/utils"

import { handleCellKey, keepKeysOnFocusedControl } from "./cell-keys"
import { DetailRenderContext, DetailRow, detailRowEstimate } from "./detail-row"
import { tabStaysInHeader } from "./header-keys"
import { gridLines, type GridLine } from "./lines"
import { legendTableTheme, panelTableTheme } from "./theme"

const modules = [
  CellStyleModule,
  ClientSideRowModelModule,
  ClientSideRowModelApiModule,
  ColumnApiModule,
  ColumnAutoSizeModule,
  PinnedRowModule,
  RowApiModule,
  RowAutoHeightModule,
  ScrollApiModule,
]

const themes = { table: panelTableTheme, legend: legendTableTheme }

type PanelGridSort = "ascending" | "descending" | "none"

type PanelGridCellStyle = { backgroundColor: string }

export type PanelGridColumn<T> = {
  id: string
  label: string
  header?: () => ReactNode
  sort?: PanelGridSort
  onSort?: () => void
  cell: (row: T) => ReactNode
  cellStyle?: (row: T) => PanelGridCellStyle
  footer?: (index: number) => ReactNode
  width?: number
  minWidth?: number
  pinned?: boolean
  autoHeight?: boolean
  wrapHeader?: boolean
}

export type PanelGridHandle = {
  reveal: (rowId: string) => void
}

export type PanelGridDetails<T> = {
  open: ReadonlySet<string>
  render: (row: T) => ReactNode
}

type FooterLine = { id: string; footer: number }

type Item<T> = GridLine<T> | FooterLine

function isFooter<T>(item: Item<T>): item is FooterLine {
  return "footer" in item
}

export function footerRowId(index: number): string {
  return `footer:${index}`
}

type HeaderParams<T> = IHeaderParams & { spec: PanelGridColumn<T> }

function PanelHeader<T>({ eGridHeader, spec }: HeaderParams<T>) {
  const sortRef = useRef(spec.onSort)
  useLayoutEffect(() => {
    sortRef.current = spec.onSort
    if (spec.sort === undefined) eGridHeader.removeAttribute("aria-sort")
    else eGridHeader.setAttribute("aria-sort", spec.sort)
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
  return (
    <div className="flex h-full w-full min-w-0 items-center">
      {spec.header === undefined ? (
        <span className="truncate px-2">{spec.label}</span>
      ) : (
        spec.header()
      )}
    </div>
  )
}

type CellParams<T> = ICellRendererParams<Item<T>> & {
  spec: PanelGridColumn<T>
}

function PanelCell<T>({ data, spec }: CellParams<T>) {
  if (data === undefined) return null
  if (isFooter(data)) return <>{spec.footer?.(data.footer) ?? null}</>
  return <>{spec.cell(data.row)}</>
}

function isDetail<T>({ rowNode }: IsFullWidthRowParams<Item<T>>): boolean {
  const data = rowNode.data
  return data !== undefined && !isFooter(data) && data.detail
}

function cellStyleOf<T>(
  column: PanelGridColumn<T>
): ColDef<Item<T>>["cellStyle"] {
  const style = column.cellStyle
  if (style === undefined) return undefined
  return ({ data }) =>
    data === undefined || isFooter(data) ? undefined : style(data.row)
}

function fitColumnsToContent(api: GridApi, available: number) {
  if (api.isDestroyed()) return
  api.autoSizeAllColumns()
  const total = (api.getColumns() ?? []).reduce(
    (sum, column) => sum + column.getActualWidth(),
    0
  )
  if (available > 0 && total < available) api.sizeColumnsToFit(available)
}

export function PanelGrid<T>({
  ref,
  label,
  rows,
  columns,
  rowId,
  rowHeight,
  headerHeight,
  footerRows = 0,
  footerHeight = rowHeight,
  details,
  sizeToContent = false,
  height = "fill",
  look = "table",
  onLayout,
  className,
}: {
  ref?: Ref<PanelGridHandle>
  label: string
  rows: readonly T[]
  columns: readonly PanelGridColumn<T>[]
  rowId: (row: T) => string
  rowHeight: number
  headerHeight: number
  footerRows?: number
  footerHeight?: number
  details?: PanelGridDetails<T>
  sizeToContent?: boolean
  height?: "fill" | "rows"
  look?: keyof typeof themes
  onLayout?: () => void
  className?: string
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const apiRef = useRef<GridApi<Item<T>> | null>(null)
  const widthRef = useRef(0)
  const labelRef = useRef(label)
  const layoutRef = useRef(onLayout)
  const pendingRevealRef = useRef<string | null>(null)
  useLayoutEffect(() => {
    labelRef.current = label
    layoutRef.current = onLayout
  })

  const open = details?.open
  const items = useMemo<Item<T>[]>(
    () => gridLines(rows, rowId, open),
    [rows, rowId, open]
  )
  const footers = useMemo(
    () =>
      Array.from({ length: footerRows }, (_, footer): Item<T> => ({
        id: footerRowId(footer),
        footer,
      })),
    [footerRows]
  )

  const columnDefs = useMemo<ColDef<Item<T>>[]>(
    () =>
      columns.map((column) => ({
        colId: column.id,
        headerName: column.label,
        sortable: false,
        resizable: false,
        suppressMovable: true,
        pinned: column.pinned ? "left" : null,
        cellRenderer: PanelCell,
        cellRendererParams: { spec: column },
        cellStyle: cellStyleOf(column),
        headerComponent: PanelHeader,
        headerComponentParams: { spec: column },
        width: column.width,
        minWidth: column.minWidth,
        flex: column.width === undefined && !sizeToContent ? 1 : undefined,
        autoHeight: column.autoHeight,
        wrapHeaderText: column.wrapHeader,
        autoHeaderHeight: column.wrapHeader,
        suppressKeyboardEvent: keepKeysOnFocusedControl,
        suppressHeaderKeyboardEvent: ({ event }) => tabStaysInHeader(event),
      })),
    [columns, sizeToContent]
  )

  const getRowId = useCallback(
    ({ data }: GetRowIdParams<Item<T>>) => data.id,
    []
  )
  const getRowHeight = useCallback(
    ({ data }: RowHeightParams<Item<T>>) => {
      if (data === undefined) return undefined
      if (isFooter(data)) return footerHeight
      return data.detail ? detailRowEstimate : undefined
    },
    [footerHeight]
  )

  const layout = useCallback(() => {
    const run = layoutRef.current
    if (run === undefined) return
    requestAnimationFrame(() => run())
  }, [])
  const revealNow = useCallback((id: string): boolean => {
    const api = apiRef.current
    if (api === null || api.isDestroyed()) return false
    const node = api.getRowNode(id)
    if (node === undefined) return false
    api.ensureNodeVisible(node)
    return true
  }, [])
  const revealPending = useCallback(() => {
    const id = pendingRevealRef.current
    pendingRevealRef.current = null
    if (id !== null) revealNow(id)
  }, [revealNow])
  const fit = useCallback(() => {
    const api = apiRef.current
    if (api !== null && sizeToContent) {
      fitColumnsToContent(api, widthRef.current)
    }
  }, [sizeToContent])
  useEffect(() => {
    fit()
  }, [fit, items, columnDefs])

  useImperativeHandle(
    ref,
    () => ({
      reveal(id) {
        pendingRevealRef.current = revealNow(id) ? null : id
      },
    }),
    [revealNow]
  )

  const labelGrid = useCallback(() => {
    containerRef.current
      ?.querySelector("[role=grid], [role=treegrid]")
      ?.setAttribute("aria-label", labelRef.current)
  }, [])
  useEffect(labelGrid, [label, labelGrid])

  const renderDetail = details?.render as
    ((row: unknown) => ReactNode) | undefined

  return (
    <div
      ref={containerRef}
      data-panel-grid=""
      className={cn("w-full", height === "fill" && "h-full", className)}
      onKeyDownCapture={(event) =>
        handleCellKey(event.nativeEvent, event.currentTarget)
      }
    >
      <DetailRenderContext value={renderDetail}>
        <AgGridReact<Item<T>>
          modules={modules}
          theme={themes[look]}
          domLayout={height === "rows" ? "autoHeight" : "normal"}
          containerStyle={
            height === "fill" ? { height: "100%", width: "100%" } : undefined
          }
          rowData={items}
          pinnedBottomRowData={footers}
          columnDefs={columnDefs}
          getRowId={getRowId}
          rowHeight={rowHeight}
          headerHeight={headerHeight}
          getRowHeight={getRowHeight}
          isFullWidthRow={isDetail}
          fullWidthCellRenderer={DetailRow}
          suppressMovableColumns
          suppressColumnVirtualisation
          ensureDomOrder
          suppressDragLeaveHidesColumns
          animateRows={false}
          onGridReady={(event) => {
            apiRef.current = event.api
            labelGrid()
          }}
          onGridSizeChanged={(event: GridSizeChangedEvent) => {
            widthRef.current = event.clientWidth
            fit()
            layout()
          }}
          onFirstDataRendered={() => {
            fit()
            layout()
          }}
          onBodyScroll={layout}
          onViewportChanged={layout}
          onModelUpdated={layout}
          onRowDataUpdated={() => {
            revealPending()
            layout()
          }}
        />
      </DetailRenderContext>
    </div>
  )
}
