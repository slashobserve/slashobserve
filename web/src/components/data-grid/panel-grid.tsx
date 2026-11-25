import {
  CellStyleModule,
  ClientSideRowModelApiModule,
  ClientSideRowModelModule,
  ColumnApiModule,
  ColumnAutoSizeModule,
  PaginationModule,
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
  createContext,
  useCallback,
  useContext,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  type ReactNode,
  type Ref,
} from "react"

import { cn } from "@/lib/utils"

import { clickCellControl, tabStaysInCell } from "./keyboard"
import { legendTableTheme, panelTableTheme } from "./theme"

const modules = [
  CellStyleModule,
  ClientSideRowModelModule,
  ClientSideRowModelApiModule,
  ColumnApiModule,
  ColumnAutoSizeModule,
  PaginationModule,
  PinnedRowModule,
  RowApiModule,
  RowAutoHeightModule,
  ScrollApiModule,
]

const themes = { table: panelTableTheme, legend: legendTableTheme }

const detailEstimate = 64

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

export type PanelGridPage = { page: number; pageCount: number }

export type PanelGridHandle = {
  reveal: (rowId: string) => void
  goToPage: (page: number) => void
}

export type PanelGridDetails<T> = {
  open: ReadonlySet<string>
  render: (row: T) => ReactNode
}

type Item<T> =
  | { kind: "row"; id: string; row: T }
  | { kind: "detail"; id: string; row: T }
  | { kind: "footer"; id: string; index: number }

export function detailRowId(rowId: string): string {
  return `detail:${rowId}`
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
  if (data.kind === "footer") return <>{spec.footer?.(data.index) ?? null}</>
  if (data.kind === "row") return <>{spec.cell(data.row)}</>
  return null
}

type DetailRows<T> = { render: (row: T) => ReactNode }

const DetailContext = createContext<DetailRows<unknown>>({
  render: () => null,
})

function DetailRow<T>({ data, node, api }: ICellRendererParams<Item<T>>) {
  const { render } = useContext(DetailContext) as DetailRows<T>
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const element = ref.current
    if (element === null) return
    function fit() {
      const height = element?.offsetHeight ?? 0
      if (height <= 0 || height === node.rowHeight || api.isDestroyed()) return
      node.setRowHeight(height)
      api.onRowHeightChanged()
    }
    fit()
    if (typeof ResizeObserver === "undefined") return
    const observer = new ResizeObserver(fit)
    observer.observe(element)
    return () => observer.disconnect()
  }, [api, node])
  if (data === undefined || data.kind !== "detail") return null
  return (
    <div ref={ref} data-detail-row="">
      {render(data.row)}
    </div>
  )
}

function isDetail<T>({ rowNode }: IsFullWidthRowParams<Item<T>>): boolean {
  return rowNode.data?.kind === "detail"
}

function cellStyleOf<T>(
  column: PanelGridColumn<T>
): ColDef<Item<T>>["cellStyle"] {
  const style = column.cellStyle
  if (style === undefined) return undefined
  return ({ data }) => (data?.kind === "row" ? style(data.row) : undefined)
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
  pageSize = null,
  onPageChange,
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
  pageSize?: number | null
  onPageChange?: (page: PanelGridPage) => void
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
  const pageRef = useRef(onPageChange)
  useLayoutEffect(() => {
    labelRef.current = label
    layoutRef.current = onLayout
    pageRef.current = onPageChange
  })

  const open = details?.open
  const items = useMemo(() => {
    const out: Item<T>[] = []
    for (const row of rows) {
      const id = rowId(row)
      out.push({ kind: "row", id, row })
      if (open?.has(id)) out.push({ kind: "detail", id: detailRowId(id), row })
    }
    return out
  }, [rows, rowId, open])
  const footers = useMemo(
    () =>
      Array.from({ length: footerRows }, (_, index): Item<T> => ({
        kind: "footer",
        id: footerRowId(index),
        index,
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
        suppressKeyboardEvent: ({ event }) =>
          clickCellControl(event) || tabStaysInCell(event),
        suppressHeaderKeyboardEvent: ({ event }) => tabStaysInCell(event),
      })),
    [columns, sizeToContent]
  )

  const getRowId = useCallback(
    ({ data }: GetRowIdParams<Item<T>>) => data.id,
    []
  )
  const getRowHeight = useCallback(
    ({ data }: RowHeightParams<Item<T>>) => {
      if (data?.kind === "footer") return footerHeight
      if (data?.kind === "detail") return detailEstimate
      return undefined
    },
    [footerHeight]
  )

  const layout = useCallback(() => {
    const run = layoutRef.current
    if (run === undefined) return
    requestAnimationFrame(() => run())
  }, [])
  const reportPage = useCallback(() => {
    const api = apiRef.current
    if (api === null || api.isDestroyed()) return
    pageRef.current?.({
      page: api.paginationGetCurrentPage(),
      pageCount: Math.max(1, api.paginationGetTotalPages()),
    })
  }, [])
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
        const api = apiRef.current
        if (api === null || api.isDestroyed()) return
        const node = api.getRowNode(id)
        if (node === undefined || node.rowIndex === null) return
        if (pageSize !== null) {
          api.paginationGoToPage(
            Math.floor(node.rowIndex / api.paginationGetPageSize())
          )
        }
        api.ensureNodeVisible(node)
      },
      goToPage(page) {
        apiRef.current?.paginationGoToPage(page)
      },
    }),
    [pageSize]
  )

  const labelGrid = useCallback(() => {
    containerRef.current
      ?.querySelector("[role=grid], [role=treegrid]")
      ?.setAttribute("aria-label", labelRef.current)
  }, [])
  useEffect(labelGrid, [label, labelGrid])

  const detailRows = useMemo(
    () => ({ render: details?.render ?? (() => null) }) as DetailRows<unknown>,
    [details?.render]
  )

  return (
    <div
      ref={containerRef}
      data-panel-grid=""
      className={cn("w-full", height === "fill" && "h-full", className)}
    >
      <DetailContext value={detailRows}>
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
          pagination={pageSize !== null}
          paginationPageSize={pageSize ?? undefined}
          suppressPaginationPanel
          paginationPageSizeSelector={false}
          suppressMovableColumns
          suppressColumnVirtualisation
          ensureDomOrder
          suppressDragLeaveHidesColumns
          animateRows={false}
          onGridReady={(event) => {
            apiRef.current = event.api
            labelGrid()
            reportPage()
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
          onPaginationChanged={() => {
            reportPage()
            layout()
          }}
          onBodyScroll={layout}
          onViewportChanged={layout}
          onModelUpdated={layout}
          onRowDataUpdated={layout}
        />
      </DetailContext>
    </div>
  )
}
