import { useMemo, useState } from "react"
import { ArrowDownIcon, ArrowUpIcon, EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"
import { cn } from "@/lib/utils"

import type { LinkScope, ResolvedLink } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  cellLinks,
  cellOf,
  tableModel,
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

function MoreLinks({ links }: { links: ResolvedLink[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            aria-label="More links"
          />
        }
      >
        <EllipsisIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-max">
        {links.map((link, index) => (
          <DropdownMenuItem
            key={`${index}:${link.href}`}
            render={<a {...anchorProps(link)} />}
            className="justify-between gap-3"
          >
            {link.title}
            {link.newTab ? <NewTabMark /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function BodyCell({
  column,
  row,
  scope,
}: {
  column: TableColumn
  row: ModelRow
  scope: LinkScope
}) {
  const value = row.values[column.key]
  const cell = cellOf(column, value)
  const [first, ...more] = cellLinks(column, row, cell, scope)
  const Icon =
    cell.word && cell.level && cell.level !== "base"
      ? thresholdIcons[cell.level]
      : null
  const content = (
    <>
      {Icon ? <Icon className="size-3 shrink-0" aria-hidden="true" /> : null}
      {cell.text}
      {cell.word ? <span className="sr-only">, {cell.word}</span> : null}
    </>
  )
  const style = cell.color ? { color: cell.color } : undefined
  return (
    <TableCell
      data-column={column.label}
      data-threshold={cell.level ?? undefined}
      title={cell.word || undefined}
      className={cn(
        "px-2 py-1",
        column.type === "number" && "text-right tabular-nums",
        value === null && "text-muted-foreground"
      )}
    >
      {first ? (
        <span className="inline-flex items-center gap-1">
          <a
            {...anchorProps(first)}
            title={first.title}
            style={style}
            className="inline-flex items-center gap-1 underline decoration-border underline-offset-4 hover:decoration-current"
          >
            {content}
            {first.newTab ? <NewTabMark /> : null}
          </a>
          {more.length > 0 ? <MoreLinks links={more} /> : null}
        </span>
      ) : cell.color ? (
        <span className="inline-flex items-center gap-1" style={style}>
          {content}
        </span>
      ) : (
        content
      )}
    </TableCell>
  )
}

export const TablePanel: PanelRenderer = ({
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
      {/* The shadcn Table wrapper adds its own overflow container, which breaks the sticky header */}
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
          {visible.map((row, index) => (
            <TableRow
              key={index}
              className="[contain-intrinsic-size:auto_33px] [content-visibility:auto]"
            >
              {model.columns.map((column) => (
                <BodyCell
                  key={column.key}
                  column={column}
                  row={row}
                  scope={links}
                />
              ))}
            </TableRow>
          ))}
        </TableBody>
        {model.footer.length > 0 ? (
          <TableFooter className="sticky bottom-0 z-10 border-t-0 bg-card shadow-[0_-1px_0_var(--border)]">
            {model.footer.map((footer) => (
              <TableRow key={footer.label} className="hover:bg-transparent">
                {model.columns.map((column, index) => (
                  <TableCell
                    key={column.key}
                    className={cn(
                      "px-2 py-1",
                      column.type === "number" && "text-right tabular-nums"
                    )}
                  >
                    {index === 0 ? (
                      <span className="flex items-center justify-between gap-2">
                        <span>{footer.label}</span>
                        {footer.cells[0] !== null ? (
                          <span className="tabular-nums">
                            {footer.cells[0]}
                          </span>
                        ) : null}
                      </span>
                    ) : (
                      footer.cells[index]
                    )}
                  </TableCell>
                ))}
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
