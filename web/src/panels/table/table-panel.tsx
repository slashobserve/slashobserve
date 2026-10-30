import { memo, useMemo, useState } from "react"
import { ArrowDownIcon, ArrowUpIcon } from "lucide-react"

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
  cellLinks,
  cellOf,
  cellSpecAt,
  gaugeFractionAt,
  rowFill,
  sparklineAt,
  tableModel,
  type Cell,
  type TableColumn,
  type TableRow as ModelRow,
} from "./table-model"

const maxRows = 1000

type Sort = { key: string; direction: "ascending" | "descending" } | null

function compareValues(a: unknown, b: unknown): number {
  if (a === null || a === undefined)
    return b === null || b === undefined ? 0 : 1
  if (b === null || b === undefined) return -1
  if (typeof a === "number" && typeof b === "number") return a - b
  return String(a).localeCompare(String(b), "en", { numeric: true })
}

type RowProps = {
  columns: TableColumn[]
  row: ModelRow
  fields: FieldResolver
  scope: LinkScope
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
}: Omit<RowProps, "columns"> & {
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
  column,
  row,
  fields,
  scope,
  rowColor,
}: Omit<RowProps, "columns"> & {
  column: TableColumn
  rowColor: string | null
}) {
  const value = row.values[column.key]
  const cell = cellOf(fields, column, row)
  const spec = cellSpecAt(fields, column, row)
  const ownFill = spec.type === "background" ? cell.color : null
  const frame = useFrame(rowColor ?? ownFill)
  const filled = frame.fill !== null
  return (
    <TableCell
      data-column={column.label}
      data-cell={spec.type}
      data-threshold={cell.level ?? undefined}
      title={cell.word || undefined}
      style={
        filled
          ? { backgroundColor: frame.fill ?? undefined, color: frame.text }
          : undefined
      }
      className={cn(
        "px-2 py-1",
        column.type === "number" && "text-right tabular-nums",
        spec.wrap && "break-words whitespace-normal",
        value === null && !filled && "text-muted-foreground"
      )}
    >
      <CellContent
        column={column}
        row={row}
        fields={fields}
        scope={scope}
        spec={spec}
        cell={cell}
        filled={filled}
      />
    </TableCell>
  )
}

const BodyRow = memo(function BodyRow({
  columns,
  row,
  fields,
  scope,
}: RowProps) {
  const rowColor = rowFill(fields, columns, row)
  return (
    <TableRow>
      {columns.map((column) => (
        <BodyCell
          key={column.key}
          column={column}
          row={row}
          fields={fields}
          scope={scope}
          rowColor={rowColor}
        />
      ))}
    </TableRow>
  )
})

const TablePanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  links,
  width,
  height,
}) => {
  const model = useMemo(
    () => tableModel(frames, fields, display),
    [frames, fields, display]
  )
  const [chosen, setSort] = useState<Sort>(null)
  const showHeader = display.header !== false
  const sort = showHeader ? chosen : null

  const rows = useMemo(() => {
    if (!sort) return model.rows
    const factor = sort.direction === "ascending" ? 1 : -1
    return [...model.rows].sort((a, b) => {
      const av = a.values[sort.key]
      const bv = b.values[sort.key]
      const missing = (v: unknown) => v === null || v === undefined
      if (missing(av) || missing(bv)) return compareValues(av, bv)
      return compareValues(av, bv) * factor
    })
  }, [model.rows, sort])

  function toggleSort(key: string) {
    setSort((current) => {
      if (current?.key !== key) return { key, direction: "ascending" }
      if (current.direction === "ascending") {
        return { key, direction: "descending" }
      }
      return null
    })
  }

  const visible = rows.slice(0, maxRows)

  return (
    <div
      role="region"
      aria-label={`${panel.title} table`}
      tabIndex={0}
      className="overflow-auto text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
      style={{ width, height }}
    >
      <table data-slot="table" className="w-full caption-bottom">
        <TableHeader
          className={
            showHeader
              ? "sticky top-0 z-10 bg-card shadow-[0_1px_0_var(--border)]"
              : "sr-only"
          }
        >
          <TableRow className="hover:bg-transparent">
            {model.columns.map((column) => {
              const active = sort?.key === column.key ? sort : null
              const numeric = column.type === "number"
              return (
                <TableHead
                  key={column.key}
                  aria-sort={
                    showHeader ? (active?.direction ?? "none") : undefined
                  }
                  className={cn("h-8 px-2", numeric && "text-right")}
                >
                  {showHeader ? (
                    <button
                      type="button"
                      onClick={() => toggleSort(column.key)}
                      className={cn(
                        "inline-flex items-center gap-1 rounded-sm font-medium outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
                        numeric && "flex-row-reverse"
                      )}
                    >
                      {column.label}
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
                </TableHead>
              )
            })}
          </TableRow>
        </TableHeader>
        <TableBody>
          {visible.map((row) => (
            <BodyRow
              key={row.key}
              columns={model.columns}
              row={row}
              fields={fields}
              scope={links}
            />
          ))}
        </TableBody>
        {model.footer.length > 0 ? (
          <TableFooter className="sticky bottom-0 z-10 border-t-0 bg-card shadow-[0_-1px_0_var(--border)]">
            {model.footer.map((footer) => (
              <TableRow key={footer.label} className="hover:bg-transparent">
                {model.columns.map((column, index) => {
                  const className = cn(
                    "px-2 py-1",
                    column.type === "number" && "text-right tabular-nums"
                  )
                  return index === 0 ? (
                    <TableHead
                      key={column.key}
                      scope="row"
                      className={cn(className, "h-auto")}
                    >
                      <span className="flex items-center justify-between gap-2">
                        <span>{footer.label}</span>
                        {footer.cells[0] !== null ? (
                          <span className="tabular-nums">
                            {footer.cells[0]}
                          </span>
                        ) : null}
                      </span>
                    </TableHead>
                  ) : (
                    <TableCell key={column.key} className={className}>
                      {footer.cells[index]}
                    </TableCell>
                  )
                })}
              </TableRow>
            ))}
          </TableFooter>
        ) : null}
      </table>
      {rows.length > maxRows ? (
        <p className="px-2 py-1.5 text-muted-foreground">
          Showing {maxRows.toLocaleString("en")} of{" "}
          {rows.length.toLocaleString("en")} rows
        </p>
      ) : null}
    </div>
  )
}

export default TablePanel
