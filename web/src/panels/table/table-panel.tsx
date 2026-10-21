import { useMemo, useState } from "react"
import { ArrowDownIcon, ArrowUpIcon } from "lucide-react"

import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatTimeValue } from "@/lib/time"
import { commonDecimals, formatValue } from "@/lib/units"
import { cn } from "@/lib/utils"

import { toTable, type TableColumn } from "../frames"
import type { PanelRenderer } from "../types"

const maxRows = 1000

type Sort = { key: string; direction: "ascending" | "descending" } | null

function compareValues(a: unknown, b: unknown): number {
  if (a === null || a === undefined)
    return b === null || b === undefined ? 0 : 1
  if (b === null || b === undefined) return -1
  if (typeof a === "number" && typeof b === "number") return a - b
  return String(a).localeCompare(String(b), "en", { numeric: true })
}

function cellText(
  column: TableColumn,
  value: unknown,
  unit: Parameters<typeof formatValue>[1],
  decimals: number | undefined
): string {
  if (value === null || value === undefined) return "-"
  if (column.type === "number" && typeof value === "number") {
    return formatValue(value, unit, decimals)
  }
  if (column.type === "time" && typeof value === "number") {
    return formatTimeValue(String(value))
  }
  if (typeof value === "object") return JSON.stringify(value)
  return String(value)
}

export const TablePanel: PanelRenderer = ({ panel, frames, width, height }) => {
  const table = useMemo(() => toTable(frames), [frames])
  const [sort, setSort] = useState<Sort>(null)

  const columnDecimals = useMemo(() => {
    const decimals = new Map<string, number | undefined>()
    for (const column of table.columns) {
      if (column.type !== "number") continue
      decimals.set(
        column.key,
        commonDecimals(
          table.rows.map((row) => row[column.key] as number | null),
          panel.unit,
          panel.decimals
        )
      )
    }
    return decimals
  }, [table, panel.unit, panel.decimals])

  const rows = useMemo(() => {
    if (!sort) return table.rows
    const factor = sort.direction === "ascending" ? 1 : -1
    return [...table.rows].sort((a, b) => {
      const av = a[sort.key]
      const bv = b[sort.key]
      const missing = (v: unknown) => v === null || v === undefined
      if (missing(av) || missing(bv)) return compareValues(av, bv)
      return compareValues(av, bv) * factor
    })
  }, [table.rows, sort])

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
      {/* The shadcn Table wrapper adds its own overflow container, which breaks the sticky header */}
      <table data-slot="table" className="w-full caption-bottom">
        <TableHeader className="sticky top-0 z-10 bg-card shadow-[0_1px_0_var(--border)]">
          <TableRow className="hover:bg-transparent">
            {table.columns.map((column) => {
              const active = sort?.key === column.key ? sort : null
              const numeric = column.type === "number"
              return (
                <TableHead
                  key={column.key}
                  aria-sort={active?.direction ?? "none"}
                  className={cn("h-8 px-2", numeric && "text-right")}
                >
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
                </TableHead>
              )
            })}
          </TableRow>
        </TableHeader>
        <TableBody>
          {visible.map((row, index) => (
            <TableRow key={index}>
              {table.columns.map((column) => {
                const value = row[column.key]
                const numeric = column.type === "number"
                return (
                  <TableCell
                    key={column.key}
                    className={cn(
                      "px-2 py-1",
                      numeric && "text-right tabular-nums",
                      value === null && "text-muted-foreground"
                    )}
                  >
                    {cellText(
                      column,
                      value,
                      panel.unit,
                      columnDecimals.get(column.key)
                    )}
                  </TableCell>
                )
              })}
            </TableRow>
          ))}
        </TableBody>
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
