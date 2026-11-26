import {
  ClientSideRowModelApiModule,
  ColumnApiModule,
  ClientSideRowModelModule,
  RowApiModule,
  RowAutoHeightModule,
  RowStyleModule,
  ScrollApiModule,
  type CellMouseOverEvent,
  type GridApi,
  type GridReadyEvent,
  type ColDef,
  type RowHeightParams,
  type GetRowIdParams,
  type ICellRendererParams,
  type IsFullWidthRowParams,
  type IHeaderParams,
  type ProcessRowParams,
  type RowClassRules,
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

import {
  focusEntryCell,
  handleCellKey,
  keepKeysOnFocusedControl,
  leaveGridOnTab,
} from "./cell-keys"
import { DetailRenderContext, DetailRow, detailRowEstimate } from "./detail-row"
import { drawnRows } from "./grid-dom"
import { gridLayout } from "./layout"
import { gridLines, type GridLine } from "./lines"
import {
  NarrowRow,
  NarrowRowsContext,
  narrowRowEstimate,
  type FullWidthRows,
  type NarrowRows,
} from "./narrow-row"
import { gridTheme } from "./theme"
import { TruncatedText } from "./truncated-text"
import { useFillHeight } from "./use-fill-height"
import { useNarrow } from "./use-narrow"
import { useRemeasureRows } from "./use-remeasure-rows"
import { useWidth } from "./use-width"

const modules = [
  ClientSideRowModelModule,
  ColumnApiModule,
  RowAutoHeightModule,
  RowStyleModule,
  ClientSideRowModelApiModule,
  RowApiModule,
  ScrollApiModule,
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

export type DataGridRemoval = {
  rowId: string
  control: string
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
  const content = column.cell(data.row)
  const plainText =
    column.autoHeight !== true &&
    (typeof content === "string" || typeof content === "number")
  return (
    <div
      className={cn(
        "flex min-w-0 items-center leading-normal",
        column.autoHeight === true ? "whitespace-normal" : "h-full",
        column.align === "right" && "justify-end text-right"
      )}
    >
      {plainText ? <TruncatedText>{content}</TruncatedText> : content}
    </div>
  )
}

type HeaderParams = IHeaderParams & {
  headerCell?: () => ReactNode
  align: "left" | "right"
  direction: DataGridSort["direction"] | null
  onSort?: () => void
  hidden: boolean
}

function SortHeader({
  displayName,
  headerCell,
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

const highlightClass = "bg-selection!"
const selectedClass = "row-selected"

function FullWidthRow<T>(params: ICellRendererParams<GridLine<T>>) {
  return params.data?.detail === true ? (
    <DetailRow {...params} />
  ) : (
    <NarrowRow {...params} />
  )
}

function narrowHeight<T>({ data }: RowHeightParams<GridLine<T>>) {
  return data?.detail === true ? detailRowEstimate : narrowRowEstimate
}

function everyRowFullWidth() {
  return true
}

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
  selectedRowIds,
  revealRowId = null,
  keepScrollOnPrepend = false,
  detail,
  compact = false,
  rowAttributes,
  bare = false,
  empty,
  fullWidthRow,
  busy = false,
  focusAfterRemoving = null,
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
  selectedRowIds?: ReadonlySet<string>
  revealRowId?: string | null
  keepScrollOnPrepend?: boolean
  detail?: DataGridDetail<T>
  compact?: boolean
  rowAttributes?: (row: T) => Readonly<Record<string, string>>
  bare?: boolean
  empty?: ReactNode
  fullWidthRow?: FullWidthRows<T>
  busy?: boolean
  focusAfterRemoving?: DataGridRemoval | null
  ref?: Ref<HTMLDivElement>
  className?: string
}) {
  const narrow = useNarrow()
  const containerRef = useRef<HTMLDivElement>(null)
  const hides = allColumns.some((column) => column.hideBelow !== undefined)
  const showsEmpty = rows.length === 0 && empty !== undefined
  const width = useWidth(containerRef, hides && !narrow && !showsEmpty)
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
  const rowAttributesRef = useRef(rowAttributes)
  const highlightRef = useRef(onHighlightRow)
  const sortRef = useRef(onSort)
  const nameRef = useRef({ label, labelledBy })
  useImperativeHandle(ref, () => containerRef.current as HTMLDivElement)
  useLayoutEffect(() => {
    nameRef.current = { label, labelledBy }
    highlightRef.current = onHighlightRow
    sortRef.current = onSort
    rowAttributesRef.current = rowAttributes
  })
  const sorts = onSort !== undefined
  const sortKey = sort?.key ?? null
  const sortDirection = sort?.direction ?? null
  const open = detail?.open
  const hasDetail = detail !== undefined
  const lines = useMemo(() => gridLines(rows, rowId, open), [rows, rowId, open])
  const getRowId = useCallback(
    ({ data }: GetRowIdParams<GridLine<T>>) => data.id,
    []
  )
  const rowClassRules = useMemo<RowClassRules<GridLine<T>>>(
    () => ({
      [highlightClass]: ({ node }) =>
        highlightedRowId !== null && node.id === highlightedRowId,
      [selectedClass]: ({ node }) =>
        node.id !== undefined && selectedRowIds?.has(node.id) === true,
    }),
    [highlightedRowId, selectedRowIds]
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
        : hasDetail || fullWidthRow !== undefined
          ? ({ rowNode }: IsFullWidthRowParams<GridLine<T>>) =>
              rowNode.data !== undefined &&
              (rowNode.data.detail ||
                fullWidthRow?.is(rowNode.data.row) === true)
          : undefined,
    [narrow, hasDetail, fullWidthRow]
  )
  const getRowHeight = useMemo(
    () =>
      narrow
        ? narrowHeight
        : hasDetail || fullWidthRow !== undefined
          ? ({ data }: RowHeightParams<GridLine<T>>) =>
              data?.detail === true
                ? detailRowEstimate
                : data !== undefined && fullWidthRow?.is(data.row)
                  ? narrowRowEstimate
                  : undefined
          : undefined,
    [narrow, hasDetail, fullWidthRow]
  )
  const renderDetail = detail?.render as
    ((row: unknown) => ReactNode) | undefined
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
        suppressKeyboardEvent: keepKeysOnFocusedControl,
        cellRenderer: GridCell,
        cellRendererParams: { column },
        headerComponent: SortHeader,
        headerComponentParams: {
          headerCell: column.headerCell,
          align: column.align ?? "left",
          hidden: column.action === true,
          direction: sortKey === column.id ? sortDirection : null,
          onSort:
            column.sortable && sorts
              ? () => sortRef.current?.(column.id)
              : undefined,
        },
        ...(narrow
          ? { flex: 1, minWidth: 1 }
          : {
              width: column.width,
              minWidth: column.minWidth,
              flex: column.width === undefined ? (column.flex ?? 1) : undefined,
              autoHeight: column.autoHeight,
            }),
      })),
    [columns, narrow, sorts, sortKey, sortDirection]
  )

  const apiRef = useRef<GridApi<GridLine<T>> | null>(null)
  const revealRef = useRef(revealRowId)
  const layoutRef = useRef(layout)
  useLayoutEffect(() => {
    revealRef.current = revealRowId
    layoutRef.current = layout
  })
  useRemeasureRows(
    containerRef,
    apiRef,
    !narrow && !hasDetail && fullWidthRow === undefined && !showsEmpty
  )
  const reveal = useCallback(() => {
    const id = revealRef.current
    const api = apiRef.current
    if (id === null || api === null || api.isDestroyed()) return
    if (layoutRef.current === "normal") {
      const node = api.getRowNode(id)
      if (node !== undefined) api.ensureNodeVisible(node)
      return
    }
    containerRef.current
      ?.querySelector(`[role=row][row-id="${CSS.escape(id)}"]`)
      ?.scrollIntoView?.({ block: "nearest" })
  }, [])
  useEffect(reveal, [revealRowId, reveal])

  const anchorRef = useRef<{
    id: string
    offset: number
    rowTop: number
  } | null>(null)
  const keepRef = useRef(keepScrollOnPrepend)
  useLayoutEffect(() => {
    keepRef.current = keepScrollOnPrepend
  })
  const bodyViewport = useCallback(
    () => containerRef.current?.querySelector<HTMLElement>(".ag-grid-viewport"),
    []
  )
  const anchorFirstRow = useCallback(() => {
    const api = apiRef.current
    anchorRef.current = null
    if (!keepRef.current || layoutRef.current !== "normal") return
    if (api === null || api.isDestroyed()) return
    const top = api.getVerticalPixelRange().top
    const node = api.getDisplayedRowAtIndex(api.getFirstDisplayedRowIndex())
    if (top <= 0 || node?.id == null || node.rowTop === null) return
    anchorRef.current = {
      id: node.id,
      offset: node.rowTop - top,
      rowTop: node.rowTop,
    }
  }, [])
  const restoreAnchor = useCallback(() => {
    const api = apiRef.current
    const anchor = anchorRef.current
    if (anchor === null || api === null || api.isDestroyed()) return
    const node = api.getRowNode(anchor.id)
    const viewport = bodyViewport()
    if (node?.rowTop == null || node.rowTop < anchor.rowTop) {
      anchorRef.current = null
      return
    }
    if (node.rowTop === anchor.rowTop || viewport == null) return
    viewport.scrollTop = node.rowTop - anchor.offset
    viewport.dispatchEvent(new Event("scroll"))
  }, [bodyViewport])

  const removingId = focusAfterRemoving?.rowId ?? null
  const removingControl = focusAfterRemoving?.control ?? null
  const removingRef = useRef<(DataGridRemoval & { rowIndex: number }) | null>(
    null
  )
  useEffect(() => {
    const api = apiRef.current
    const rowIndex =
      removingId === null || api === null || api.isDestroyed()
        ? null
        : (api.getRowNode(removingId)?.rowIndex ?? null)
    removingRef.current =
      removingId === null || removingControl === null || rowIndex === null
        ? null
        : { rowId: removingId, control: removingControl, rowIndex }
  }, [removingId, removingControl])
  const focusNeighbourOfRemoved = useCallback(() => {
    const removing = removingRef.current
    const api = apiRef.current
    const container = containerRef.current
    if (removing === null || api === null || container === null) return
    if (api.isDestroyed() || api.getRowNode(removing.rowId) !== undefined) {
      return
    }
    removingRef.current = null
    const active = container.ownerDocument.activeElement
    const focusIsFree =
      active === null ||
      active === container.ownerDocument.body ||
      container.contains(active)
    const count = api.getDisplayedRowCount()
    if (!focusIsFree || count === 0) return
    const rowIndex = Math.min(removing.rowIndex, count - 1)
    const node = api.getDisplayedRowAtIndex(rowIndex)
    const row = node?.id == null ? undefined : drawnRows(container).get(node.id)
    const control = row?.querySelector<HTMLElement>(removing.control)
    if (control == null) return
    const column = control.closest("[col-id]")?.getAttribute("col-id")
    if (column != null) api.setFocusedCell(rowIndex, column)
    control.focus()
  }, [])
  const onRowDataUpdated = useCallback(() => {
    restoreAnchor()
    focusNeighbourOfRemoved()
  }, [restoreAnchor, focusNeighbourOfRemoved])

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
  const onGridReady = useCallback(
    (event: GridReadyEvent<GridLine<T>>) => {
      apiRef.current = event.api
      labelGrid()
    },
    [labelGrid]
  )

  if (showsEmpty) return <>{empty}</>

  return (
    <div
      ref={containerRef}
      data-layout={narrow ? "narrow" : "wide"}
      data-dom-layout={layout}
      tabIndex={-1}
      aria-busy={busy || undefined}
      className={cn(
        "w-full text-[13px] outline-none [&_.ag-cell-auto-height]:flex! [&_.ag-cell-auto-height]:items-center! [&_.ag-cell-auto-height>.ag-cell-wrapper]:w-full [&_.row-selected]:bg-selection!",
        bare &&
          "[&_.ag-root-wrapper]:rounded-none! [&_.ag-root-wrapper]:border-0! [&_.ag-styled-root]:[--ag-background-color:transparent]! [&_.ag-styled-root]:[--ag-chrome-background-color:transparent]! [&_.ag-styled-root]:[--ag-header-background-color:transparent]!",
        className
      )}
      style={layout === "normal" ? { height } : undefined}
      onPointerOver={(event) => {
        if (
          event.target instanceof Element &&
          event.target.closest(".ag-header") !== null
        )
          highlight(null)
      }}
      onPointerLeave={() => onHighlightRow?.(null)}
      onKeyDownCapture={(event) =>
        handleCellKey(event.nativeEvent, event.currentTarget)
      }
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
            tabToNextCell={leaveGridOnTab}
            focusGridInnerElement={focusEntryCell}
            isFullWidthRow={isFullWidthRow}
            fullWidthCellRenderer={FullWidthRow}
            getRowHeight={getRowHeight}
            rowClassRules={rowClassRules}
            processRowPostCreate={processRowPostCreate}
            onCellMouseOver={onCellMouseOver}
            onGridReady={onGridReady}
            onFirstDataRendered={reveal}
            onBodyScroll={anchorFirstRow}
            onRowDataUpdated={onRowDataUpdated}
          />
        </NarrowRowsContext>
      </DetailRenderContext>
    </div>
  )
}
