import { useEffect, useMemo, useState, type Ref } from "react"

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
} from "./legend"
import { legendHeaders } from "./legend-model"
import {
  legendNameMinWidth,
  legendValueWidths,
  type TextMeasure,
} from "./legend-width"

let measureContext: CanvasRenderingContext2D | null = null
const averageGlyphEm = 0.6

function legendTextMeasure(rem: number): TextMeasure {
  measureContext ??= document.createElement("canvas").getContext("2d")
  const context = measureContext
  const size = 0.75 * rem
  if (context === null) return (text) => text.length * averageGlyphEm * size
  const family = getComputedStyle(document.documentElement).fontFamily
  return (text, weight) => {
    context.font = `${weight} ${size}px ${family}`
    return context.measureText(text).width
  }
}

const newMeasure = (): typeof legendTextMeasure => (rem) =>
  legendTextMeasure(rem)

export function useLegendTextMeasure(): typeof legendTextMeasure {
  const [measure, setMeasure] = useState(newMeasure)
  useEffect(() => {
    if (document.fonts.status === "loaded") return
    let current = true
    void document.fonts.ready.then(() => {
      if (current) setMeasure(newMeasure)
    })
    return () => {
      current = false
    }
  }, [])
  return measure
}

type Props = {
  ref?: Ref<HTMLDivElement>
  items: LegendItem[]
  reducers: readonly string[]
  headers?: readonly string[]
  cells: string[][]
  sortBy?: string
  sortDesc?: boolean
  hidden: ReadonlySet<string>
  placement: LegendPlacement
  available: number
  width: number
  onToggle: (key: string) => void
  onHover?: LegendHover
  more?: number
}

type LegendRow =
  | { kind: "item"; item: LegendItem; cells: readonly string[] }
  | { kind: "more"; count: number }

const legendRowId = (row: LegendRow) =>
  row.kind === "item" ? `item:${row.item.key}` : "more"

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
  const columns = useMemo<PanelGridColumn<LegendRow>[]>(
    () => [
      {
        id: "name",
        label: "Series",
        header: () => <span className="sr-only">Series</span>,
        sort: sortBy === "name" ? sorted : undefined,
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
                <span className="truncate">{row.item.label}</span>
              </button>
            </span>
          ),
      },
      ...reducers.map((reducer, column): PanelGridColumn<LegendRow> => ({
        id: `value:${column}`,
        label: names[column],
        header: () => (
          <span className="w-full px-1.5 text-right font-medium whitespace-nowrap text-muted-foreground tabular-nums">
            {names[column]}
          </span>
        ),
        sort: sortBy === reducer ? sorted : undefined,
        width: valueWidths[column],
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
