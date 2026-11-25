import {
  useCallback,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactElement,
} from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  BracesIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ClockIcon,
  HashIcon,
  ToggleLeftIcon,
  TypeIcon,
} from "lucide-react"

import { drawnColumns, drawnRows } from "@/components/data-grid/grid-dom"
import {
  PanelGrid,
  footerRowId,
  type PanelGridColumn,
  type PanelGridHandle,
  type PanelGridPage,
} from "@/components/data-grid/panel-grid"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { oneClickTarget, type FieldAction } from "../actions/field-actions"
import {
  runFieldAction,
  useStableActionRunner,
  type StableActionRunner,
} from "../actions/action-runner"
import { rectWithin, surfaceOf, useAnchorPublisher } from "../anchor-layout"
import type { Rect } from "../anchors"
import { resolveColor } from "../colors"
import { cssColorToRgb } from "../contrast"
import type { FieldResolver, LinkScope, ResolvedLink } from "../field-config"
import { oneClickProps } from "@/dashboards/dashboard-links"
import type { PanelRenderer } from "../types"
import type { AnchorColumn, AnchorRow, TableAnchorLayout } from "./anchors"
import { ActionsCell } from "./cells/actions"
import { fillTextColor, type CellSpec } from "./cells/cell-model"
import { GaugeCell } from "./cells/gauge"
import { ImageCell } from "./cells/image"
import { JsonCell } from "./cells/json"
import { LinksCell } from "./cells/links"
import { MarkdownCell } from "./cells/markdown"
import { PillCell } from "./cells/pill"
import { SparklineCell } from "./cells/sparkline"
import { TextCell } from "./cells/text"
import {
  clearFilter,
  filterRows,
  toggleValue,
  type ColumnFilters,
} from "./column-filter"
import { CellTooltip } from "./cell-tooltip"
import { ColumnFilterMenu } from "./column-filter-menu"
import { InspectCell } from "./inspect-cell"
import {
  alignOf,
  columnWidths,
  footerRowPx,
  frozenCount,
  headerPx,
  initialSort,
  pageSizeFor,
  rowHeightOf,
  rowHeightPx,
  rowSizing,
  tooltipSideOf,
  typeWord,
  type Align,
  type RowHeight,
  type Sort,
} from "./table-layout"
import {
  cellActions,
  cellLinks,
  cellOf,
  cellScope,
  cellSpecAt,
  cellTooltipAt,
  footerRows,
  gaugeFractionAt,
  rowFill,
  selectFrames,
  sparklineAt,
  tableModel,
  type Cell,
  type FooterRow,
  type NestedRows,
  type TableColumn,
  type TableRow as ModelRow,
} from "./table-model"

const maxRows = 1000
const expandWidth = 32
const expandColumnId = "expand"
const nestedRowPx = 25

const typeIcons = {
  number: HashIcon,
  string: TypeIcon,
  time: ClockIcon,
  bool: ToggleLeftIcon,
  other: BracesIcon,
}

const bodyPadding: Record<RowHeight, string> = {
  sm: "py-1",
  md: "py-2",
  lg: "py-3",
}

const alignClasses: Record<Align, string> = {
  left: "text-left",
  center: "text-center",
  right: "text-right",
}

const justifyClasses: Record<Align, string> = {
  left: "justify-start",
  center: "justify-center",
  right: "justify-end",
}

type ColumnView = {
  column: TableColumn
  align: Align
  inspect: boolean
  filterable: boolean
  hideHeader: boolean
  wrapHeader: boolean
  description: string | undefined
}

function compareValues(a: unknown, b: unknown): number {
  if (a === null || a === undefined)
    return b === null || b === undefined ? 0 : 1
  if (b === null || b === undefined) return -1
  if (typeof a === "number" && typeof b === "number") return a - b
  return String(a).localeCompare(String(b), "en", { numeric: true })
}

function cellFill(
  fields: FieldResolver,
  views: readonly ColumnView[],
  column: TableColumn,
  row: ModelRow
): string | null {
  const whole = rowFill(
    fields,
    views.map((view) => view.column),
    row
  )
  if (whole !== null) return whole
  const spec = cellSpecAt(fields, column, row)
  return spec.type === "background" ? cellOf(fields, column, row).color : null
}

function useFillText(fill: string | null): string | undefined {
  useTheme()
  return fillTextColor(fill, (value) => cssColorToRgb(resolveColor(value)))
}

function growsWith(
  fields: FieldResolver,
  display: Record<string, unknown>,
  column: TableColumn,
  row: ModelRow | undefined
): boolean {
  if (row === undefined) return false
  const spec = cellSpecAt(fields, column, row)
  return (
    rowSizing(display, spec.dynamicHeight).grows ||
    spec.wrap ||
    spec.type === "json"
  )
}

type CellProps = {
  row: ModelRow
  fields: FieldResolver
  scope: LinkScope
}

function CellContent({
  column,
  row,
  fields,
  links,
  actions,
  onAction,
  reason,
  spec,
  cell,
  filled,
  grows,
}: Pick<CellProps, "row" | "fields"> & {
  column: TableColumn
  links: ResolvedLink[]
  actions: FieldAction[]
  onAction: (action: FieldAction) => void
  reason: string | null
  spec: CellSpec
  cell: Cell
  filled: boolean
  grows: boolean
}) {
  const value = row.values[column.key]
  switch (spec.type) {
    case "gauge":
      return (
        <GaugeCell
          cell={cell}
          spec={spec}
          gauge={gaugeFractionAt(fields, column, row)}
        />
      )
    case "sparkline": {
      const spark = sparklineAt(fields, column, row)
      if (spark) {
        return <SparklineCell cell={cell} spec={spec} {...spark} />
      }
      break
    }
    case "json":
      return <JsonCell value={value} text={cell.text} />
    case "image":
      return (
        <ImageCell
          value={value}
          alt={spec.imageAlt ?? column.label}
          title={spec.imageTitle}
        />
      )
    case "links":
      return <LinksCell cell={cell} links={links} />
    case "pill":
      return <PillCell value={value} />
    case "markdown":
      return <MarkdownCell value={value} grows={grows} />
    case "actions":
      return <ActionsCell cell={cell} actions={actions} onAction={onAction} />
  }
  return (
    <TextCell
      cell={cell}
      links={links}
      actions={actions}
      onAction={onAction}
      reason={reason}
      tinted={!filled && spec.type !== "background"}
    />
  )
}

function BodyCell({
  view,
  views,
  row,
  fields,
  scope,
  display,
  autoHeight,
  runner,
}: CellProps & {
  view: ColumnView
  views: readonly ColumnView[]
  display: Record<string, unknown>
  autoHeight: boolean
  runner: StableActionRunner
}) {
  const { column } = view
  const value = row.values[column.key]
  const cell = cellOf(fields, column, row)
  const spec = cellSpecAt(fields, column, row)
  const sizing = rowSizing(display, spec.dynamicHeight)
  const tooltip = cellTooltipAt(fields, column, row)
  const fill = cellFill(fields, views, column, row)
  const filled = fill !== null
  const fillText = useFillText(fill)
  const links = cellLinks(fields, column, row, cell, scope)
  const actions = cellActions(fields, column, row, cell, scope)
  function runAction(action: FieldAction) {
    runFieldAction(runner, action, cellScope(fields, column, row, cell, scope))
  }
  const target = oneClickTarget(links, actions)
  const oneClick =
    spec.type === "links" || spec.type === "actions" || target === null
      ? {}
      : target.kind === "link"
        ? oneClickProps(links)
        : runner.reason === null
          ? oneClickProps(links, () => runAction(target.action))
          : {}
  const content = (
    <CellContent
      column={column}
      row={row}
      fields={fields}
      links={links}
      actions={actions}
      onAction={runAction}
      reason={runner.reason}
      spec={spec}
      cell={cell}
      filled={filled}
      grows={sizing.grows}
    />
  )
  const inner = view.inspect ? (
    <span className={cn("flex items-center gap-1", justifyClasses[view.align])}>
      {content}
      <InspectCell label={column.label} value={value} text={cell.text} />
    </span>
  ) : (
    content
  )
  const wraps = spec.wrap || sizing.grows
  return (
    <div
      data-column={column.label}
      data-cell={spec.type}
      {...oneClick}
      data-threshold={cell.level ?? undefined}
      title={cell.word || undefined}
      style={fillText === undefined ? undefined : { color: fillText }}
      className={cn(
        "flex w-full min-w-0 items-center px-2 data-one-click:cursor-pointer",
        autoHeight ? bodyPadding[sizing.padding] : "h-full",
        justifyClasses[view.align],
        alignClasses[view.align],
        column.type === "number" && "tabular-nums",
        value === null && !filled && "text-muted-foreground"
      )}
    >
      <div
        className={cn(
          "min-w-0",
          wraps
            ? "break-words whitespace-normal"
            : "overflow-hidden text-ellipsis whitespace-nowrap"
        )}
      >
        <CellTooltip
          text={tooltip?.text ?? null}
          side={tooltipSideOf(tooltip?.placement ?? "auto")}
        >
          {sizing.maxHeight === null ? (
            inner
          ) : (
            <div
              className="overflow-hidden"
              style={{ maxHeight: sizing.maxHeight }}
            >
              {inner}
            </div>
          )}
        </CellTooltip>
      </div>
    </div>
  )
}

function NestedTable({
  nested,
  fields,
  zone,
  label,
}: {
  nested: NestedRows
  fields: FieldResolver
  zone: string | undefined
  label: string
}) {
  const model = useMemo(
    () => tableModel([nested.frame], fields, zone),
    [nested.frame, fields, zone]
  )
  const rows = useMemo(() => model.rows.slice(0, maxRows), [model.rows])
  const columns = useMemo(
    () =>
      model.columns.map((column): PanelGridColumn<ModelRow> => ({
        id: column.key,
        label: column.label,
        header: () => (
          <span
            className={cn(
              "w-full truncate px-2 font-medium",
              column.type === "number" ? "text-right" : "text-left"
            )}
          >
            {column.label}
          </span>
        ),
        cell: (row) => (
          <span
            className={cn(
              "flex h-full w-full items-center truncate px-2",
              column.type === "number" && "justify-end tabular-nums"
            )}
          >
            {cellOf(fields, column, row).text}
          </span>
        ),
      })),
    [model.columns, fields]
  )
  return (
    <PanelGrid
      label={label}
      rows={rows}
      columns={columns}
      rowId={rowKey}
      rowHeight={nestedRowPx}
      headerHeight={nested.headers ? nestedRowPx : 0}
      height="rows"
    />
  )
}

function ExpandButton({
  row,
  label,
  expanded,
  detailId,
  onToggle,
}: {
  row: ModelRow
  label: string
  expanded: boolean
  detailId: string
  onToggle: (key: string) => void
}) {
  const Chevron = expanded ? ChevronDownIcon : ChevronRightIcon
  return (
    <span className="flex h-full items-center px-1">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={detailId}
        aria-label={`${expanded ? "Collapse" : "Expand"} ${label}`}
        onClick={() => onToggle(row.key)}
        className="inline-flex size-6 items-center justify-center rounded-sm outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Chevron className="size-4" aria-hidden="true" />
      </button>
    </span>
  )
}

function HeaderTooltip({
  text,
  children,
}: {
  text: string | undefined
  children: ReactElement
}) {
  if (text === undefined) return children
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent side="top" role="tooltip">
        {text}
      </TooltipContent>
    </Tooltip>
  )
}

function HeaderContent({
  view,
  sort,
  showHeader,
  showTypeIcons,
  rows,
  fields,
  kept,
  onSort,
  onToggle,
  onClear,
}: {
  view: ColumnView
  sort: Sort | null
  showHeader: boolean
  showTypeIcons: boolean
  rows: ModelRow[]
  fields: FieldResolver
  kept: ReadonlySet<string>
  onSort: (key: string) => void
  onToggle: (key: string, value: string) => void
  onClear: (key: string) => void
}) {
  const { column } = view
  const active = sort?.key === column.key ? sort : null
  const TypeMark = typeIcons[column.type]
  const textOf = useCallback(
    (row: ModelRow) => cellOf(fields, column, row).text,
    [fields, column]
  )
  return (
    <span
      className={cn(
        "flex w-full min-w-0 items-center gap-1 px-2",
        justifyClasses[view.align],
        alignClasses[view.align],
        view.align === "right" && "flex-row-reverse",
        view.wrapHeader && "py-1"
      )}
    >
      {showHeader ? (
        <HeaderTooltip text={view.description}>
          <button
            type="button"
            onClick={() => onSort(column.key)}
            className={cn(
              "inline-flex min-w-0 items-center gap-1 rounded-sm font-medium outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
              view.align === "right" && "flex-row-reverse",
              view.wrapHeader
                ? "text-left break-words whitespace-normal"
                : "truncate"
            )}
          >
            {showTypeIcons ? (
              <TypeMark
                className="size-3 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            ) : null}
            <span className={view.hideHeader ? "sr-only" : undefined}>
              {column.label}
            </span>
            {showTypeIcons ? (
              <span className="sr-only">, {typeWord(column.type)}</span>
            ) : null}
            {active?.direction === "ascending" ? (
              <ArrowUpIcon className="size-3 shrink-0" aria-hidden="true" />
            ) : null}
            {active?.direction === "descending" ? (
              <ArrowDownIcon className="size-3 shrink-0" aria-hidden="true" />
            ) : null}
          </button>
        </HeaderTooltip>
      ) : (
        column.label
      )}
      {showHeader && view.filterable ? (
        <ColumnFilterMenu
          label={column.label}
          rows={rows}
          textOf={textOf}
          kept={kept}
          onToggle={(value) => onToggle(column.key, value)}
          onClear={() => onClear(column.key)}
        />
      ) : null}
    </span>
  )
}

function FooterCell({
  footer,
  index,
  view,
}: {
  footer: FooterRow
  index: number
  view: ColumnView
}) {
  const className = cn(
    "flex h-full w-full items-center px-2",
    justifyClasses[view.align],
    view.column.type === "number" && "tabular-nums"
  )
  if (index > 0) {
    return <span className={className}>{footer.cells[index]}</span>
  }
  return (
    <span
      className={cn(className, "justify-between gap-2 font-medium")}
      data-footer-label={footer.label}
    >
      <span>{footer.label}</span>
      {footer.cells[0] !== null ? (
        <span className="tabular-nums">{footer.cells[0]}</span>
      ) : null}
    </span>
  )
}

const noKept: ReadonlySet<string> = new Set()

const rowKey = (row: ModelRow) => row.key

const TablePanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  zone,
  meta,
  links,
  width,
  height,
}) => {
  const shownFrames = useMemo(
    () => selectFrames(frames, display.frameIndex),
    [frames, display.frameIndex]
  )
  const model = useMemo(
    () => tableModel(shownFrames, fields, zone),
    [shownFrames, fields, zone]
  )
  const [pick, setPick] = useState<{ from: string; sort: Sort | null } | null>(
    null
  )
  const [filters, setFilters] = useState<ColumnFilters>(new Map())
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set())
  const [paging, setPaging] = useState<PanelGridPage | null>(null)
  const gridRef = useRef<PanelGridHandle>(null)
  const regionRef = useRef<HTMLDivElement>(null)
  const { runner, dialog } = useStableActionRunner(panel.id, meta)
  const baseId = useId()
  const expandable = useMemo(
    () => model.rows.some((row) => row.nested !== null),
    [model.rows]
  )
  const showHeader = display.header !== false
  const rowHeight = rowHeightOf(display)
  const frozen = frozenCount(display)
  const paginate = display.pagination === true
  const showTypeIcons = display.typeIcons === true

  const initial = useMemo(
    () => initialSort(model.columns, display),
    [model.columns, display]
  )
  const initialId = initial ? `${initial.key}:${initial.direction}` : ""
  const sort = showHeader && pick?.from === initialId ? pick.sort : initial

  const layout = useMemo(
    () =>
      columnWidths(
        model.columns.map(({ config }) => ({
          width: config.width,
          minWidth: config.minWidth,
        })),
        width,
        frozen > 0
      ),
    [model.columns, width, frozen]
  )

  const views = useMemo(
    () =>
      model.columns.map((column): ColumnView => ({
        column,
        align: alignOf(column.config.align, column.type),
        inspect: column.config.inspect,
        filterable: column.config.filterable,
        hideHeader: column.config.hideHeader,
        wrapHeader: column.config.wrapHeader,
        description: column.config.fieldDescription,
      })),
    [model.columns]
  )

  const filtered = useMemo(() => {
    const byKey = new Map(model.columns.map((column) => [column.key, column]))
    return filterRows(model.rows, filters, (key, row) => {
      const column = byKey.get(key)
      return column ? cellOf(fields, column, row).text : ""
    })
  }, [model.rows, model.columns, filters, fields])

  const sorted = useMemo(() => {
    if (!sort) return filtered
    const factor = sort.direction === "ascending" ? 1 : -1
    return [...filtered].sort((a, b) => {
      const av = a.values[sort.key]
      const bv = b.values[sort.key]
      const missing = (v: unknown) => v === null || v === undefined
      if (missing(av) || missing(bv)) return compareValues(av, bv)
      return compareValues(av, bv) * factor
    })
  }, [filtered, sort])

  const footer = useMemo(
    () => footerRows(model.columns, filtered, display),
    [model.columns, filtered, display]
  )

  const pageSize = pageSizeFor(height, rowHeight, {
    header: showHeader,
    footerRows: footer.length,
  })
  const rows = useMemo(
    () => (paginate ? sorted : sorted.slice(0, maxRows)),
    [paginate, sorted]
  )

  const toggleSort = useCallback(
    (key: string) => {
      let next: Sort | null = null
      if (sort?.key !== key) next = { key, direction: "ascending" }
      else if (sort.direction === "ascending") {
        next = { key, direction: "descending" }
      }
      setPick({ from: initialId, sort: next })
      gridRef.current?.goToPage(0)
    },
    [sort, initialId]
  )

  const toggleOpen = useCallback((key: string) => {
    setOpen((current) => {
      const next = new Set(current)
      if (!next.delete(key)) next.add(key)
      return next
    })
  }, [])

  const changeFilters = useCallback((next: ColumnFilters) => {
    setFilters(next)
    gridRef.current?.goToPage(0)
  }, [])

  const firstRow = rows[0]
  const columns = useMemo(() => {
    const firstView = views[0]
    const labelOf = (row: ModelRow) =>
      firstView ? cellOf(fields, firstView.column, row).text : "row"
    const expand: PanelGridColumn<ModelRow>[] = expandable
      ? [
          {
            id: expandColumnId,
            label: "Details",
            header: () => <span className="sr-only">Details</span>,
            width: expandWidth,
            pinned: frozen > 0,
            cell: (row) =>
              row.nested ? (
                <ExpandButton
                  row={row}
                  label={labelOf(row)}
                  expanded={open.has(row.key)}
                  detailId={`${baseId}-${row.key}`}
                  onToggle={toggleOpen}
                />
              ) : null,
          },
        ]
      : []
    return [
      ...expand,
      ...views.map((view, index): PanelGridColumn<ModelRow> => {
        const key = view.column.key
        const autoHeight = growsWith(fields, display, view.column, firstRow)
        return {
          id: key,
          label: view.column.label,
          header: () => (
            <HeaderContent
              view={view}
              sort={sort}
              showHeader={showHeader}
              showTypeIcons={showTypeIcons}
              rows={model.rows}
              fields={fields}
              kept={filters.get(key) ?? noKept}
              onSort={toggleSort}
              onToggle={(column, value) =>
                changeFilters(toggleValue(filters, column, value))
              }
              onClear={(column) => changeFilters(clearFilter(filters, column))}
            />
          ),
          sort: showHeader
            ? sort?.key === key
              ? sort.direction
              : "none"
            : undefined,
          onSort: showHeader ? () => toggleSort(key) : undefined,
          cell: (row) => (
            <BodyCell
              view={view}
              views={views}
              row={row}
              fields={fields}
              scope={links}
              display={display}
              autoHeight={autoHeight}
              runner={runner}
            />
          ),
          cellStyle: (row) => ({
            backgroundColor: cellFill(fields, views, view.column, row) ?? "",
          }),
          footer: (row) => (
            <FooterCell footer={footer[row]} index={index} view={view} />
          ),
          width: layout?.widths[index],
          pinned: index < frozen,
          autoHeight,
          wrapHeader: showHeader && view.wrapHeader,
        }
      }),
    ]
  }, [
    views,
    expandable,
    frozen,
    open,
    baseId,
    toggleOpen,
    fields,
    display,
    firstRow,
    sort,
    showHeader,
    showTypeIcons,
    model.rows,
    filters,
    toggleSort,
    changeFilters,
    links,
    runner,
    footer,
    layout,
  ])

  const details = useMemo(
    () => ({
      open,
      render: (row: ModelRow) => {
        if (row.nested === null) return null
        const label = views[0]
          ? cellOf(fields, views[0].column, row).text
          : "row"
        return (
          <div id={`${baseId}-${row.key}`} className="pl-[18px]">
            <NestedTable
              nested={row.nested}
              fields={fields}
              zone={zone}
              label={`${label} rows`}
            />
          </div>
        )
      },
    }),
    [open, views, fields, baseId, zone]
  )

  const rowCells = useMemo(
    () =>
      new Map(
        rows.map((row) => [
          row.key,
          views.map(({ column }) => cellOf(fields, column, row).text),
        ])
      ),
    [rows, views, fields]
  )
  const reveal = useCallback((key: string) => gridRef.current?.reveal(key), [])
  const publish = useAnchorPublisher("table")
  const measure = useCallback(() => {
    const region = regionRef.current
    if (region === null) return
    publish(
      tableAnchorLayout(region, {
        body: { width, height },
        views,
        showHeader,
        rowCells,
        reveal,
      })
    )
  }, [publish, width, height, views, showHeader, rowCells, reveal])
  useLayoutEffect(measure, [measure])

  const page = paging ?? {
    page: 0,
    pageCount: Math.max(1, Math.ceil(rows.length / pageSize)),
  }

  return (
    <div className="flex flex-col text-xs" style={{ width, height }}>
      <div
        ref={regionRef}
        role="region"
        aria-label={`${panel.title} table`}
        className="min-h-0 flex-1"
      >
        <PanelGrid
          ref={gridRef}
          label={panel.title}
          rows={rows}
          columns={columns}
          rowId={rowKey}
          rowHeight={rowHeightPx(rowHeight)}
          headerHeight={showHeader ? headerPx : 0}
          footerRows={footer.length}
          footerHeight={footerRowPx}
          pageSize={paginate ? pageSize : null}
          onPageChange={setPaging}
          details={expandable ? details : undefined}
          sizeToContent={layout === null}
          onLayout={measure}
        />
      </div>
      {!paginate && sorted.length > maxRows ? (
        <p className="shrink-0 px-2 py-1.5 text-muted-foreground">
          Showing {maxRows.toLocaleString("en")} of{" "}
          {sorted.length.toLocaleString("en")} rows
        </p>
      ) : null}
      {paginate ? (
        <div className="flex h-10 shrink-0 items-center justify-end gap-2 border-t border-border px-2">
          <p role="status" className="text-muted-foreground tabular-nums">
            Page {page.page + 1} of {page.pageCount}
          </p>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Previous page"
            disabled={page.page === 0}
            onClick={() => gridRef.current?.goToPage(page.page - 1)}
          >
            <ChevronLeftIcon aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Next page"
            disabled={page.page >= page.pageCount - 1}
            onClick={() => gridRef.current?.goToPage(page.page + 1)}
          >
            <ChevronRightIcon aria-hidden="true" />
          </Button>
        </div>
      ) : null}
      {dialog}
    </div>
  )
}

function visibleIn(rect: Rect, top: number, bottom: number): boolean {
  const middle = rect.y + rect.height / 2
  return middle >= top && middle <= bottom
}

function spanOf(rects: readonly Rect[]): Pick<Rect, "y" | "height"> | null {
  if (rects.length === 0) return null
  const top = Math.min(...rects.map((rect) => rect.y))
  const bottom = Math.max(...rects.map((rect) => rect.y + rect.height))
  return { y: top, height: bottom - top }
}

function tableAnchorLayout(
  region: HTMLElement,
  {
    body,
    views,
    showHeader,
    rowCells,
    reveal,
  }: {
    body: { width: number; height: number }
    views: readonly ColumnView[]
    showHeader: boolean
    rowCells: ReadonlyMap<string, readonly string[]>
    reveal: (key: string) => void
  }
): TableAnchorLayout {
  const root = surfaceOf(region)
  const area = rectWithin(region, root)
  const headerCells = drawnColumns(region)
  const columnRects = views.flatMap(({ column }) => {
    const cell = headerCells.get(column.key)
    return cell === undefined ? [] : [{ column, rect: rectWithin(cell, root) }]
  })
  const columns: AnchorColumn[] = columnRects.map(({ column, rect }) => ({
    name: column.label,
    left: rect.x,
    width: rect.width,
    dimension: column.type !== "number",
  }))
  const header = showHeader ? spanOf(columnRects.map(({ rect }) => rect)) : null
  const drawnRowElements = drawnRows(region)
  const footerRow = drawnRowElements.get(footerRowId(0))
  const top = header === null ? area.y : header.y + header.height
  const bottom =
    footerRow === undefined
      ? area.y + area.height
      : rectWithin(footerRow, root).y
  const rows: AnchorRow[] = []
  const drawn = new Set<string>()
  for (const [key, element] of drawnRowElements) {
    const cells = rowCells.get(key)
    if (cells === undefined) continue
    const rect = rectWithin(element, root)
    if (!visibleIn(rect, top, bottom)) continue
    drawn.add(key)
    rows.push({ top: rect.y, height: rect.height, cells })
  }
  rows.sort((a, b) => a.top - b.top)
  const elsewhere = Array.from(rowCells).flatMap(([key, cells]) =>
    drawn.has(key) ? [] : [{ key, cells }]
  )
  return {
    body,
    header,
    columns,
    rows,
    elsewhere,
    reveal,
  }
}

export default TablePanel
