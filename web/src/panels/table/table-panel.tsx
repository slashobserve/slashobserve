import { memo, useCallback, useMemo, useState, type CSSProperties } from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  BracesIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ClockIcon,
  HashIcon,
  ToggleLeftIcon,
  TypeIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { resolveColor } from "../colors"
import { cssColorToRgb } from "../contrast"
import type { FieldResolver, LinkScope } from "../field-config"
import type { PanelRenderer } from "../types"
import { fillTextColor, type CellSpec } from "./cells/cell-model"
import { GaugeCell } from "./cells/gauge"
import { ImageCell } from "./cells/image"
import { JsonCell } from "./cells/json"
import { LinksCell } from "./cells/links"
import { PillCell } from "./cells/pill"
import { SparklineCell } from "./cells/sparkline"
import { TextCell } from "./cells/text"
import {
  clearFilter,
  filterRows,
  toggleValue,
  type ColumnFilters,
} from "./column-filter"
import { ColumnFilterMenu } from "./column-filter-menu"
import { InspectCell } from "./inspect-cell"
import {
  alignOf,
  columnWidths,
  frozenCount,
  initialSort,
  pageRows,
  pageSizeFor,
  rowHeightOf,
  stickyOffsets,
  typeWord,
  type Align,
  type RowHeight,
  type Sort,
} from "./table-layout"
import {
  cellLinks,
  cellOf,
  cellSpecAt,
  footerRows,
  gaugeFractionAt,
  rowFill,
  sparklineAt,
  tableModel,
  type Cell,
  type TableColumn,
  type TableRow as ModelRow,
} from "./table-model"

const maxRows = 1000

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
  left: number | null
}

function stickyStyle(left: number | null): CSSProperties | undefined {
  return left === null ? undefined : { position: "sticky", left }
}

function compareValues(a: unknown, b: unknown): number {
  if (a === null || a === undefined)
    return b === null || b === undefined ? 0 : 1
  if (b === null || b === undefined) return -1
  if (typeof a === "number" && typeof b === "number") return a - b
  return String(a).localeCompare(String(b), "en", { numeric: true })
}

type CellProps = {
  row: ModelRow
  fields: FieldResolver
  scope: LinkScope
}

type RowShared = CellProps & {
  padding: RowHeight
  fixed: boolean
}

type CellFrame = { fill: string | null; text: string | undefined }

function useFrame(color: string | null): CellFrame {
  useTheme()
  return {
    fill: color,
    text: fillTextColor(color, (value) => cssColorToRgb(resolveColor(value))),
  }
}

function CellContent({
  column,
  row,
  fields,
  scope,
  spec,
  cell,
  filled,
}: CellProps & {
  column: TableColumn
  spec: CellSpec
  cell: Cell
  filled: boolean
}) {
  const value = row.values[column.key]
  const links = cellLinks(fields, column, row, cell, scope)
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
  }
  return (
    <TextCell
      cell={cell}
      links={links}
      tinted={!filled && spec.type !== "background"}
    />
  )
}

function BodyCell({
  view,
  row,
  fields,
  scope,
  rowColor,
  padding,
  fixed,
}: RowShared & {
  view: ColumnView
  rowColor: string | null
}) {
  const { column } = view
  const value = row.values[column.key]
  const cell = cellOf(fields, column, row)
  const spec = cellSpecAt(fields, column, row)
  const ownFill = spec.type === "background" ? cell.color : null
  const frame = useFrame(rowColor ?? ownFill)
  const filled = frame.fill !== null
  const content = (
    <CellContent
      column={column}
      row={row}
      fields={fields}
      scope={scope}
      spec={spec}
      cell={cell}
      filled={filled}
    />
  )
  return (
    <TableCell
      data-column={column.label}
      data-cell={spec.type}
      data-threshold={cell.level ?? undefined}
      title={cell.word || undefined}
      style={{
        ...stickyStyle(view.left),
        ...(filled
          ? { backgroundColor: frame.fill ?? undefined, color: frame.text }
          : undefined),
      }}
      className={cn(
        "px-2",
        bodyPadding[padding],
        alignClasses[view.align],
        column.type === "number" && "tabular-nums",
        spec.wrap && "break-words whitespace-normal",
        fixed && !spec.wrap && "overflow-hidden text-ellipsis",
        view.left !== null && !filled && "bg-card",
        value === null && !filled && "text-muted-foreground"
      )}
    >
      {view.inspect ? (
        <span
          className={cn("flex items-center gap-1", justifyClasses[view.align])}
        >
          {content}
          <InspectCell label={column.label} value={value} text={cell.text} />
        </span>
      ) : (
        content
      )}
    </TableCell>
  )
}

const BodyRow = memo(function BodyRow({
  views,
  ...shared
}: RowShared & { views: ColumnView[] }) {
  const rowColor = rowFill(
    shared.fields,
    views.map((view) => view.column),
    shared.row
  )
  return (
    <TableRow>
      {views.map((view) => (
        <BodyCell
          key={view.column.key}
          view={view}
          rowColor={rowColor}
          {...shared}
        />
      ))}
    </TableRow>
  )
})

function HeaderCell({
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
    <TableHead
      aria-sort={showHeader ? (active?.direction ?? "none") : undefined}
      style={stickyStyle(view.left)}
      className={cn(
        "h-8 px-2",
        alignClasses[view.align],
        view.left !== null && "z-[1] bg-card"
      )}
    >
      <span
        className={cn(
          "flex items-center gap-1",
          justifyClasses[view.align],
          view.align === "right" && "flex-row-reverse"
        )}
      >
        {showHeader ? (
          <button
            type="button"
            onClick={() => onSort(column.key)}
            className={cn(
              "inline-flex items-center gap-1 rounded-sm font-medium outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
              view.align === "right" && "flex-row-reverse"
            )}
          >
            {showTypeIcons ? (
              <TypeMark
                className="size-3 text-muted-foreground"
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
              <ArrowUpIcon className="size-3" aria-hidden="true" />
            ) : null}
            {active?.direction === "descending" ? (
              <ArrowDownIcon className="size-3" aria-hidden="true" />
            ) : null}
          </button>
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
    </TableHead>
  )
}

const noKept: ReadonlySet<string> = new Set()

const TablePanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  links,
  width,
  height,
}) => {
  const model = useMemo(() => tableModel(frames, fields), [frames, fields])
  const [pick, setPick] = useState<{ from: string; sort: Sort | null } | null>(
    null
  )
  const [filters, setFilters] = useState<ColumnFilters>(new Map())
  const [requestedPage, setPage] = useState(0)
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

  const views = useMemo(() => {
    const offsets = layout ? stickyOffsets(layout.widths, frozen) : []
    return model.columns.map((column, index): ColumnView => ({
      column,
      align: alignOf(column.config.align, column.type),
      inspect: column.config.inspect,
      filterable: column.config.filterable,
      hideHeader: column.config.hideHeader,
      left: offsets[index] ?? null,
    }))
  }, [model.columns, layout, frozen])

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
  const paged = pageRows(sorted, requestedPage, pageSize)
  const visible = paginate ? paged.rows : sorted.slice(0, maxRows)

  function toggleSort(key: string) {
    let next: Sort | null = null
    if (sort?.key !== key) next = { key, direction: "ascending" }
    else if (sort.direction === "ascending") {
      next = { key, direction: "descending" }
    }
    setPick({ from: initialId, sort: next })
    setPage(0)
  }

  function changeFilters(next: ColumnFilters) {
    setFilters(next)
    setPage(0)
  }

  const fixed = layout !== null

  return (
    <div className="flex flex-col text-xs" style={{ width, height }}>
      <div
        role="region"
        aria-label={`${panel.title} table`}
        tabIndex={0}
        className="min-h-0 flex-1 overflow-auto outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
      >
        <table
          data-slot="table"
          className={cn("caption-bottom", !fixed && "w-full")}
          style={
            layout ? { tableLayout: "fixed", width: layout.total } : undefined
          }
        >
          {layout ? (
            <colgroup>
              {layout.widths.map((columnWidth, index) => (
                <col
                  key={views[index].column.key}
                  style={{ width: columnWidth }}
                />
              ))}
            </colgroup>
          ) : null}
          <TableHeader
            className={
              showHeader
                ? "sticky top-0 z-10 bg-card shadow-[0_1px_0_var(--border)]"
                : "sr-only"
            }
          >
            <TableRow className="hover:bg-transparent">
              {views.map((view) => (
                <HeaderCell
                  key={view.column.key}
                  view={view}
                  sort={sort}
                  showHeader={showHeader}
                  showTypeIcons={showTypeIcons}
                  rows={model.rows}
                  fields={fields}
                  kept={filters.get(view.column.key) ?? noKept}
                  onSort={toggleSort}
                  onToggle={(key, value) =>
                    changeFilters(toggleValue(filters, key, value))
                  }
                  onClear={(key) => changeFilters(clearFilter(filters, key))}
                />
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.map((row) => (
              <BodyRow
                key={row.key}
                views={views}
                row={row}
                fields={fields}
                scope={links}
                padding={rowHeight}
                fixed={fixed}
              />
            ))}
          </TableBody>
          {footer.length > 0 ? (
            <TableFooter className="sticky bottom-0 z-10 border-t-0 bg-card shadow-[0_-1px_0_var(--border)]">
              {footer.map((footerRow) => (
                <TableRow
                  key={footerRow.label}
                  className="hover:bg-transparent"
                >
                  {views.map((view, index) => {
                    const className = cn(
                      "px-2 py-1",
                      alignClasses[view.align],
                      view.column.type === "number" && "tabular-nums",
                      view.left !== null && "z-[1] bg-card"
                    )
                    return index === 0 ? (
                      <TableHead
                        key={view.column.key}
                        scope="row"
                        style={stickyStyle(view.left)}
                        className={cn(className, "h-auto")}
                      >
                        <span className="flex items-center justify-between gap-2">
                          <span>{footerRow.label}</span>
                          {footerRow.cells[0] !== null ? (
                            <span className="tabular-nums">
                              {footerRow.cells[0]}
                            </span>
                          ) : null}
                        </span>
                      </TableHead>
                    ) : (
                      <TableCell
                        key={view.column.key}
                        style={stickyStyle(view.left)}
                        className={className}
                      >
                        {footerRow.cells[index]}
                      </TableCell>
                    )
                  })}
                </TableRow>
              ))}
            </TableFooter>
          ) : null}
        </table>
        {!paginate && sorted.length > maxRows ? (
          <p className="px-2 py-1.5 text-muted-foreground">
            Showing {maxRows.toLocaleString("en")} of{" "}
            {sorted.length.toLocaleString("en")} rows
          </p>
        ) : null}
      </div>
      {paginate ? (
        <div className="flex h-10 shrink-0 items-center justify-end gap-2 border-t border-border px-2">
          <p role="status" className="text-muted-foreground tabular-nums">
            Page {paged.page + 1} of {paged.pageCount}
          </p>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Previous page"
            disabled={paged.page === 0}
            onClick={() => setPage(paged.page - 1)}
          >
            <ChevronLeftIcon aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Next page"
            disabled={paged.page >= paged.pageCount - 1}
            onClick={() => setPage(paged.page + 1)}
          >
            <ChevronRightIcon aria-hidden="true" />
          </Button>
        </div>
      ) : null}
    </div>
  )
}

export default TablePanel
