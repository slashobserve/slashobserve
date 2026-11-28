import { useMemo, type Ref } from "react"

import {
  PanelGrid,
  type PanelGridColumn,
} from "@/components/data-grid/panel-grid"
import { cn } from "@/lib/utils"

import {
  LegendSwatch,
  hoverHandlers,
  legendTableFit,
  rootFontSize,
  rowButtonClass,
  padRem,
  rowRem,
  type LegendHover,
  type LegendItem,
  type LegendPlacement,
  LegendName,
  useLegendNames,
} from "./legend"
import { useLegendTextMeasure } from "./legend-measure"
import { legendHeaders } from "./legend-model"
import {
  legendNameMinWidth,
  legendNameRoom,
  legendValueWidths,
} from "./legend-width"

type Props = {
  ref?: Ref<HTMLDivElement>
  items: LegendItem[]
  reducers: readonly string[]
  headers?: readonly string[]
  cells: string[][]
  sortBy?: string | undefined
  sortDesc?: boolean
  hidden: ReadonlySet<string>
  placement: LegendPlacement
  available: number
  width: number
  onToggle: (key: string) => void
  onHover?: LegendHover | undefined
  more?: number
}

type LegendRow =
  | { kind: "item"; item: LegendItem; cells: readonly string[] }
  | { kind: "more"; count: number }

const legendRowId = (row: LegendRow) =>
  row.kind === "item" ? `item:${row.item.key}` : "more"

function widthOf(width: number | undefined): { width?: number } {
  return width === undefined ? {} : { width }
}

export function LegendTable({
  ref,
  items,
  reducers,
  headers,
  cells,
  sortBy,
  sortDesc = false,
  hidden,
  placement,
  available,
  width,
  onToggle,
  onHover,
  more = 0,
}: Props) {
  const measure = useLegendTextMeasure()
  const sorted = sortDesc ? "descending" : "ascending"
  const bottom = placement === "bottom"
  const rem = rootFontSize()
  const rowPx = rowRem * rem
  const total = items.length + more
  const rows = useMemo<LegendRow[]>(
    () => [
      ...items.map((item, index): LegendRow => ({
        kind: "item",
        item,
        cells: cells[index] ?? [],
      })),
      ...(more > 0 ? [{ kind: "more", count: more } as const] : []),
    ],
    [items, cells, more]
  )
  const fit = legendTableFit(available, rem, rows.length)
  const scrolls = bottom && fit.scrolls
  const footer = bottom && fit.footer
  const names = legendHeaders(reducers, headers)
  const valueWidths = useMemo(
    () =>
      legendValueWidths(names, cells, measure(rem), rem).map((value) =>
        Math.ceil(value)
      ),
    [names, cells, measure, rem]
  )
  const shownNames = useLegendNames(
    items,
    placement === "right"
      ? legendNameRoom(
          width,
          rem,
          valueWidths.reduce((sum, value) => sum + value, 0)
        )
      : null
  )
  const shown = useMemo(
    () => new Map(items.map((item, index) => [item.key, shownNames[index]])),
    [items, shownNames]
  )
  const columns = useMemo<PanelGridColumn<LegendRow>[]>(
    () => [
      {
        id: "name",
        label: "Series",
        header: () => <span className="sr-only">Series</span>,
        ...(sortBy === "name" ? { sort: sorted } : {}),
        minWidth: legendNameMinWidth(rem),
        cell: (row) =>
          row.kind === "more" ? (
            <span className="flex h-full items-center px-1.5 text-xs text-muted-foreground">
              +{row.count} more
            </span>
          ) : (
            <span className="flex h-full min-w-0 items-center">
              <button
                type="button"
                aria-label={row.item.label}
                aria-pressed={!hidden.has(row.item.key)}
                data-style={row.item.style}
                onClick={() => onToggle(row.item.key)}
                className={rowButtonClass(!hidden.has(row.item.key))}
                {...hoverHandlers(row.item.key, onHover)}
              >
                <LegendSwatch
                  color={row.item.color}
                  line={row.item.line}
                  style={row.item.style}
                />
                <LegendName
                  label={row.item.label}
                  text={shown.get(row.item.key) ?? row.item.label}
                />
              </button>
            </span>
          ),
      },
      ...reducers.map((reducer, column): PanelGridColumn<LegendRow> => ({
        id: `value:${column}`,
        label: names[column] ?? "",
        header: () => (
          <span className="w-full px-1.5 text-right font-medium whitespace-nowrap text-muted-foreground tabular-nums">
            {names[column]}
          </span>
        ),
        ...(sortBy === reducer ? { sort: sorted } : {}),
        ...widthOf(valueWidths[column]),
        cell: (row) =>
          row.kind === "item" ? (
            <span
              className={cn(
                "flex h-full items-center justify-end px-1.5 text-right whitespace-nowrap text-muted-foreground tabular-nums",
                hidden.has(row.item.key) && "opacity-50"
              )}
            >
              {row.cells[column]}
            </span>
          ) : null,
      })),
    ],
    [
      reducers,
      names,
      shown,
      valueWidths,
      sortBy,
      sorted,
      rem,
      hidden,
      onToggle,
      onHover,
    ]
  )
  const contentPx = (rows.length + 1) * rowPx + padRem * rem
  return (
    <div
      ref={ref}
      role="group"
      aria-label="Series"
      className={cn("flex shrink-0 flex-col px-1", !scrolls && "pb-1.5")}
      style={
        bottom
          ? { height: scrolls ? fit.maxHeight : contentPx }
          : { width, height: contentPx, maxHeight: "100%" }
      }
    >
      <PanelGrid
        label="Series"
        look="legend"
        rows={rows}
        columns={columns}
        rowId={legendRowId}
        rowHeight={rowPx}
        headerHeight={rowPx}
        className="min-h-0 flex-1"
      />
      {footer ? (
        <p
          className="shrink-0 px-1.5 pt-0.5 text-xs text-muted-foreground"
          style={{ height: `${rowRem + padRem}rem` }}
        >
          {more > 0
            ? `${total} series, ${more} not listed`
            : `${total} series, scroll for all`}
        </p>
      ) : null}
    </div>
  )
}
