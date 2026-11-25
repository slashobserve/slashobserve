import { useEffect, useState, type Ref } from "react"

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
import type { TextMeasure } from "./legend-width"

let measureContext: CanvasRenderingContext2D | null = null

function legendTextMeasure(rem: number): TextMeasure {
  measureContext ??= document.createElement("canvas").getContext("2d")
  const context = measureContext!
  const family = getComputedStyle(document.documentElement).fontFamily
  return (text, weight) => {
    context.font = `${weight} ${0.75 * rem}px ${family}`
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
  const sorted = sortDesc ? "descending" : "ascending"
  const bottom = placement === "bottom"
  const rem = rootFontSize()
  const fit = legendTableFit(available, rem, items.length + (more > 0 ? 1 : 0))
  const overflows = bottom && fit.overflows
  const names = legendHeaders(reducers, headers)
  return (
    <div
      ref={ref}
      role="group"
      aria-label="Series"
      className={cn("shrink-0 overflow-auto px-1", !overflows && "pb-1.5")}
      style={bottom ? { maxHeight: fit.maxHeight } : { width }}
    >
      <table className="w-full border-separate border-spacing-0 text-xs">
        <thead>
          <tr>
            <th
              scope="col"
              aria-sort={sortBy === "name" ? sorted : undefined}
              className="sticky top-0 bg-background"
            >
              <span className="sr-only">Series</span>
            </th>
            {reducers.map((reducer, column) => (
              <th
                key={reducer}
                scope="col"
                aria-sort={sortBy === reducer ? sorted : undefined}
                className="sticky top-0 bg-background px-1.5 py-0.5 text-right font-medium whitespace-nowrap text-muted-foreground tabular-nums"
              >
                {names[column]}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((item, row) => {
            const visible = !hidden.has(item.key)
            return (
              <tr key={item.key} data-style={item.style}>
                <th
                  scope="row"
                  className="w-full max-w-0 min-w-16 p-0 text-left font-normal"
                >
                  <button
                    type="button"
                    aria-pressed={visible}
                    onClick={() => onToggle(item.key)}
                    className={rowButtonClass(visible)}
                    {...hoverHandlers(item.key, onHover)}
                  >
                    <LegendSwatch
                      color={item.color}
                      line={item.line}
                      style={item.style}
                    />
                    <span className="truncate">{item.label}</span>
                  </button>
                </th>
                {cells[row].map((text, column) => (
                  <td
                    key={reducers[column]}
                    className={
                      visible
                        ? "px-1.5 py-0.5 text-right whitespace-nowrap text-muted-foreground tabular-nums"
                        : "px-1.5 py-0.5 text-right whitespace-nowrap text-muted-foreground tabular-nums opacity-50"
                    }
                  >
                    {text}
                  </td>
                ))}
              </tr>
            )
          })}
          {more > 0 ? (
            <tr>
              <td
                colSpan={reducers.length + 1}
                className="px-1.5 py-0.5 text-muted-foreground"
              >
                +{more} more
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>
      {overflows ? (
        <p
          className="sticky bottom-0 bg-background px-1.5 pt-0.5 text-xs text-muted-foreground"
          style={{ height: `${rowRem + padRem}rem` }}
        >
          {items.length} series, scroll for all
        </p>
      ) : null}
    </div>
  )
}
