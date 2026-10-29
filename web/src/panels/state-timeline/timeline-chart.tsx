import { useMemo, useState, type PointerEvent } from "react"

import { formatDateTime } from "@/lib/time"
import { timeTicks } from "@/lib/time-ticks"
import { useTheme } from "@/lib/theme"
import { formatValue } from "@/lib/units"

import { resolveColor } from "../colors"
import { cssColorToRgb, readableText } from "../contrast"
import { CursorLine, useCursorLine } from "../cursor-line"
import type { SharedCursor } from "../shared-cursor"
import {
  labelWidth,
  legendHeight,
  legendItems,
  moreLabel,
  type LegendItems,
  rowGap,
  rowHeight,
  showsText,
  showValuesOf,
  type ShowValues,
  timeToX,
} from "./timeline-layout"
import {
  segmentAt,
  type Segment,
  type TimelineModel,
  type TimelineRow,
} from "./timeline-model"

const padX = 12
const padTop = 6
const xAxisHeight = 20
const tickSpacing = 72
const tickEdgeGap = 20
const textInset = 6
const cellInset = 1
const tooltipOffset = 12
const fallbackText = "var(--foreground)"

export type TimelineVariant = "segments" | "cells"

type Hover = { row: number; segment: Segment; x: number; y: number }

function textColors(
  model: TimelineModel,
  mode: ShowValues
): ReadonlyMap<string, string> {
  const colors = new Map<string, string>()
  if (mode === "never") return colors
  for (const { segments } of model.rows) {
    for (const { color } of segments) {
      if (colors.has(color)) continue
      const rgb = cssColorToRgb(resolveColor(color))
      colors.set(color, rgb ? readableText(rgb) : fallbackText)
    }
  }
  return colors
}

function RowLabels({
  rows,
  width,
  rowSize,
}: {
  rows: TimelineRow[]
  width: number
  rowSize: number
}) {
  return (
    <div aria-hidden="true" className="shrink-0" style={{ width }}>
      {rows.map((row) => (
        <div
          key={row.key}
          title={row.name}
          className="truncate pr-2 text-xs text-muted-foreground"
          style={{ height: rowSize, lineHeight: `${rowSize}px` }}
        >
          {row.name}
        </div>
      ))}
    </div>
  )
}

function TimeLabels({
  from,
  to,
  width,
}: {
  from: number
  to: number
  width: number
}) {
  const ticks = useMemo(() => {
    const count = Math.max(2, Math.round(width))
    const xs = Array.from(
      { length: count },
      (_, index) => from + ((to - from) * index) / (count - 1)
    )
    return timeTicks(xs, Math.floor(width / tickSpacing))
      .map((tick) => ({ ...tick, left: (tick.index / (count - 1)) * width }))
      .filter(
        (tick) => tick.left >= tickEdgeGap && tick.left <= width - tickEdgeGap
      )
  }, [from, to, width])
  return (
    <div
      aria-hidden="true"
      className="relative text-xs text-muted-foreground tabular-nums"
      style={{ width, height: xAxisHeight }}
    >
      {ticks.map((tick) => (
        <span
          key={tick.index}
          className="absolute top-1 -translate-x-1/2 leading-4 whitespace-nowrap"
          style={{ left: tick.left }}
        >
          {tick.label}
        </span>
      ))}
    </div>
  )
}

function Legend({ shown, more }: LegendItems) {
  return (
    <ul
      aria-label="Legend"
      className="m-0 flex list-none flex-wrap content-start gap-x-4 gap-y-0 overflow-y-auto p-0 pt-2 text-xs"
    >
      {shown.map((state) => (
        <li
          key={state.text}
          className="flex min-w-0 items-center gap-1.5 leading-[18px]"
        >
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0 rounded-[2px]"
            style={{ backgroundColor: state.color }}
          />
          <span className="truncate">{state.text}</span>
        </li>
      ))}
      {more > 0 ? (
        <li className="leading-[18px] text-muted-foreground">
          {moreLabel(more)}
        </li>
      ) : null}
    </ul>
  )
}

function Tooltip({
  name,
  segment,
  x,
  y,
  width,
  height,
}: {
  name: string
  segment: Segment
  x: number
  y: number
  width: number
  height: number
}) {
  const onRight = x > width / 2
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-40 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(y + tooltipOffset, Math.max(0, height - 88)),
        ...(onRight
          ? { right: width - x + tooltipOffset }
          : { left: x + tooltipOffset }),
      }}
    >
      <div className="mb-1 truncate text-muted-foreground">{name}</div>
      <div className="flex items-center gap-1.5 font-medium">
        <span
          aria-hidden="true"
          className="size-2.5 shrink-0 rounded-[2px]"
          style={{ backgroundColor: segment.color }}
        />
        <span className="min-w-0 truncate">{segment.text}</span>
      </div>
      <div className="mt-1 text-muted-foreground tabular-nums">
        {formatDateTime(segment.from)} to {formatDateTime(segment.to)}
      </div>
      <div className="tabular-nums">
        {formatValue(segment.to - segment.from, "dtdurationms")}
      </div>
    </div>
  )
}

export function TimelineChart({
  model,
  display,
  from,
  to,
  width,
  height,
  variant,
  cursor,
}: {
  model: TimelineModel
  display: Record<string, unknown>
  from: number
  to: number
  width: number
  height: number
  variant: TimelineVariant
  cursor: SharedCursor
}) {
  const { resolved: theme } = useTheme()
  const [hover, setHover] = useState<Hover | null>(null)
  const mode = showValuesOf(display)
  const inset = variant === "cells" ? cellInset : 0
  const colors = useMemo(
    () => textColors(model, mode),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resolveColor reads the live theme tokens, so a theme flip must recompute
    [model, mode, theme]
  )
  const rows = model.rows.length
  const segments = model.rows.reduce((sum, row) => sum + row.segments.length, 0)
  const innerWidth = width - 2 * padX
  const labels = labelWidth(innerWidth)
  const plotWidth = innerWidth - labels
  const legend = legendItems(model.states)
  const legendSpace = Math.min(
    height * 0.4,
    legendHeight(legend.labels, innerWidth)
  )
  const available = height - padTop - xAxisHeight - legendSpace
  const rowSize = rowHeight(available, rows)
  const plotHeight = rowSize * rows
  const span = Math.max(1, to - from)
  const ready = rows > 0 && plotWidth > 0 && available > 0
  const { lineRef, move: moveCursor } = useCursorLine(cursor, (x) =>
    timeToX(x, from, to, plotWidth)
  )

  function move(event: PointerEvent<SVGSVGElement>) {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left
    const y = event.clientY - box.top
    const row = Math.floor(y / rowSize)
    moveCursor(from + (x / plotWidth) * span)
    const segment =
      row >= 0 && row < rows
        ? segmentAt(model.rows[row].segments, from + (x / plotWidth) * span)
        : null
    setHover(segment ? { row, segment, x, y } : null)
  }

  return (
    <div
      data-rows={rows}
      data-segments={segments}
      data-states={model.states.length}
      className="flex flex-col"
      style={{ width, height, padding: `${padTop}px ${padX}px 0` }}
    >
      {ready ? (
        <>
          <div className="flex">
            <RowLabels rows={model.rows} width={labels} rowSize={rowSize} />
            <div className="relative" style={{ width: plotWidth }}>
              <svg
                role="img"
                aria-label={`${rows} rows of states over time`}
                width={plotWidth}
                height={plotHeight}
                className="block cursor-crosshair"
                onPointerMove={move}
                onPointerLeave={() => {
                  setHover(null)
                  moveCursor(null)
                }}
              >
                {model.rows.map((row, index) =>
                  row.segments.map((segment) => {
                    const left = ((segment.from - from) / span) * plotWidth
                    const size =
                      ((segment.to - segment.from) / span) * plotWidth
                    const drawn = Math.max(1, size - 1 - 2 * inset)
                    const top = index * rowSize
                    const bar = Math.max(1, rowSize - rowGap)
                    return (
                      <g key={`${row.key}:${segment.from}`}>
                        <rect
                          x={left + inset}
                          y={top}
                          width={drawn}
                          height={bar}
                          rx={2}
                          style={{ fill: segment.color }}
                        />
                        {showsText(mode, size, bar) ? (
                          <svg
                            x={left + inset}
                            y={top}
                            width={drawn}
                            height={bar}
                          >
                            <text
                              x={textInset}
                              y={bar / 2}
                              dominantBaseline="central"
                              fontSize={12}
                              style={{
                                fill: colors.get(segment.color) ?? fallbackText,
                              }}
                            >
                              {segment.text}
                            </text>
                          </svg>
                        ) : null}
                      </g>
                    )
                  })
                )}
              </svg>
              <CursorLine lineRef={lineRef} height={plotHeight} />
              {hover ? (
                <Tooltip
                  name={model.rows[hover.row].name}
                  segment={hover.segment}
                  x={hover.x}
                  y={hover.y}
                  width={plotWidth}
                  height={plotHeight}
                />
              ) : null}
            </div>
          </div>
          <div style={{ marginLeft: labels }}>
            <TimeLabels from={from} to={to} width={plotWidth} />
          </div>
          <Legend {...legend} />
        </>
      ) : null}
    </div>
  )
}
