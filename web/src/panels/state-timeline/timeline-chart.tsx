import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent,
} from "react"

import { formatDateTime } from "@/lib/time"
import { timeTicks } from "@/lib/time-ticks"
import { useTheme } from "@/lib/theme"
import { formatValue } from "@/lib/units"
import { cn } from "@/lib/utils"

import { resolveColor } from "../colors"
import { cssColorToRgb, readableText, type Rgb } from "../contrast"
import { CursorLine, useCursorLine } from "../cursor-line"
import { Legend, type LegendItem } from "../legend/legend"
import { LegendTable } from "../legend/legend-table"
import { rightLegendWidth } from "../legend/legend-width"
import type { SharedCursor } from "../shared-cursor"
import {
  emptySeriesState,
  useSeriesStore,
  visibleSeries,
} from "../timeseries/series-state"
import {
  barSpan,
  blendOver,
  labelWidth,
  rowHeight,
  segmentSpan,
  showsText,
  textPlacement,
  timelineDisplay,
  timeToX,
  type ShowValues,
} from "./timeline-layout"
import {
  segmentAt,
  withoutStates,
  type Segment,
  type TimelineModel,
  type TimelineRow,
} from "./timeline-model"
import { tooltipEntries, type TooltipEntry } from "./timeline-tooltip"

const padX = 12
const padTop = 6
const xAxisHeight = 20
const tickSpacing = 72
const tickEdgeGap = 20
const tooltipOffset = 12
const legendShare = 0.4
const singleTooltipHeight = 88
const listTooltipChrome = 40
const listTooltipRow = 18
const fallbackText = "var(--foreground)"
const fallbackBackdrop: Rgb = [255, 255, 255]
const noHidden: ReadonlySet<string> = new Set()

export type TimelineVariant = "segments" | "cells"

type Hover = { row: number; at: number; x: number; y: number }

function textColors(
  model: TimelineModel,
  mode: ShowValues,
  backdrop: Rgb
): ReadonlyMap<string, string> {
  const colors = new Map<string, string>()
  if (mode === "never") return colors
  for (const { segments, opacity } of model.rows) {
    for (const { color } of segments) {
      const key = `${opacity}|${color}`
      if (colors.has(key)) continue
      const rgb = cssColorToRgb(resolveColor(color))
      colors.set(
        key,
        rgb ? readableText(blendOver(rgb, backdrop, opacity)) : fallbackText
      )
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

function Swatch({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="size-2.5 shrink-0 rounded-[2px]"
      style={{ backgroundColor: color }}
    />
  )
}

function Tooltip({
  segment,
  name,
  entries,
  at,
  hovered,
  x,
  y,
  width,
  height,
  maxWidth,
  maxHeight,
}: {
  segment: Segment | null
  name: string
  entries: TooltipEntry[] | null
  at: number
  hovered: number
  x: number
  y: number
  width: number
  height: number
  maxWidth?: number
  maxHeight?: number
}) {
  const onRight = x > width / 2
  const estimate = entries
    ? listTooltipChrome + listTooltipRow * entries.length
    : singleTooltipHeight
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-40 overflow-hidden rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(y + tooltipOffset, Math.max(0, height - estimate)),
        maxWidth,
        maxHeight,
        ...(onRight
          ? { right: width - x + tooltipOffset }
          : { left: x + tooltipOffset }),
      }}
    >
      {entries ? (
        <>
          <div className="mb-1 text-muted-foreground tabular-nums">
            {formatDateTime(at)}
          </div>
          {entries.map((entry) => (
            <div
              key={entry.row}
              className={cn(
                "flex items-center gap-1.5",
                entry.row === hovered && "font-medium"
              )}
            >
              <Swatch color={entry.segment.color} />
              <span className="min-w-0 truncate text-muted-foreground">
                {entry.name}
              </span>
              <span className="ml-auto pl-2">{entry.segment.text}</span>
            </div>
          ))}
        </>
      ) : segment ? (
        <>
          <div className="mb-1 truncate text-muted-foreground">{name}</div>
          <div className="flex items-center gap-1.5 font-medium">
            <Swatch color={segment.color} />
            <span className="min-w-0 truncate">{segment.text}</span>
          </div>
          <div className="mt-1 text-muted-foreground tabular-nums">
            {formatDateTime(segment.from)} to {formatDateTime(segment.to)}
          </div>
          <div className="tabular-nums">
            {formatValue(segment.to - segment.from, "dtdurationms")}
          </div>
        </>
      ) : null}
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
  stateKey,
}: {
  model: TimelineModel
  display: Record<string, unknown>
  from: number
  to: number
  width: number
  height: number
  variant: TimelineVariant
  cursor: SharedCursor
  stateKey: string
}) {
  const { resolved: theme } = useTheme()
  const [hover, setHover] = useState<Hover | null>(null)
  const [legendHeight, setLegendHeight] = useState(0)
  const legendRef = useRef<HTMLDivElement>(null)
  const options = useMemo(() => timelineDisplay(display), [display])
  const mode = options.showValues
  const cells = variant === "cells"

  const stored = useSeriesStore((store) => store.panels[stateKey])
  const commitKeys = useSeriesStore((store) => store.commit)
  const toggleKey = useSeriesStore((store) => store.toggle)
  const keys = useMemo(
    () => model.states.map(({ text }) => text),
    [model.states]
  )
  const legendShown = options.legend !== "hidden" && keys.length > 0
  const hiddenStates = useMemo(
    () =>
      legendShown
        ? visibleSeries((stored ?? emptySeriesState).hidden, keys, true)
        : noHidden,
    [stored, keys, legendShown]
  )
  const toggle = useCallback(
    (key: string) => toggleKey(stateKey, key),
    [toggleKey, stateKey]
  )
  useLayoutEffect(() => {
    commitKeys(stateKey, keys)
  }, [commitKeys, stateKey, keys])

  const shown = useMemo(
    () => withoutStates(model, hiddenStates),
    [model, hiddenStates]
  )
  const colors = useMemo(
    () => {
      const backdrop =
        cssColorToRgb(resolveColor("var(--background)")) ?? fallbackBackdrop
      return textColors(shown, mode, backdrop)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resolveColor reads the live theme tokens, so a theme flip must recompute
    [shown, mode, theme]
  )

  const rightLegend = legendShown && options.legend === "right"
  const bottomLegend = legendShown && options.legend === "bottom"
  const legendMode = options.legendMode ?? "list"
  const legendWidth = rightLegend
    ? rightLegendWidth(null, width, options.legendWidth)
    : 0
  useLayoutEffect(() => {
    const element = legendRef.current
    if (!element || !bottomLegend) return
    const observer = new ResizeObserver(() =>
      setLegendHeight(Math.ceil(element.getBoundingClientRect().height))
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [bottomLegend, legendMode])

  const rows = shown.rows.length
  const segments = shown.rows.reduce((sum, row) => sum + row.segments.length, 0)
  const chartWidth = width - legendWidth
  const innerWidth = chartWidth - 2 * padX
  const labels = labelWidth(innerWidth)
  const plotWidth = innerWidth - labels
  const available =
    height - padTop - xAxisHeight - (bottomLegend ? legendHeight : 0)
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
    const at = from + (x / plotWidth) * span
    moveCursor(at)
    if (options.tooltip === "hidden" || row < 0 || row >= rows) {
      setHover(null)
      return
    }
    const listed =
      options.tooltip === "all"
        ? tooltipEntries(shown.rows, at, {
            sort: options.tooltipSort,
            hideZeros: options.tooltipHideZeros,
          }).entries.length > 0
        : segmentAt(shown.rows[row].segments, at) !== null
    setHover(listed ? { row, at, x, y } : null)
  }

  const items: LegendItem[] = model.states.map((state) => ({
    key: state.text,
    label: state.text,
    color: state.color,
    line: "solid",
    style: "bars",
  }))
  const legendProps = {
    ref: legendRef,
    items,
    reducers: [],
    cells: items.map(() => []),
    hidden: hiddenStates,
    placement: rightLegend ? ("right" as const) : ("bottom" as const),
    available: height * legendShare,
    width: legendWidth,
    onToggle: toggle,
  }
  const bar = barSpan(rowSize, options.rowHeight)
  const hovered = hover ? shown.rows[hover.row] : null
  const hoveredEntries =
    hover && options.tooltip === "all"
      ? tooltipEntries(shown.rows, hover.at, {
          sort: options.tooltipSort,
          hideZeros: options.tooltipHideZeros,
        }).entries
      : null

  return (
    <div
      data-rows={rows}
      data-segments={segments}
      data-states={model.states.length}
      className={cn("flex", rightLegend ? "flex-row" : "flex-col")}
      style={{ width, height }}
    >
      <div
        className="flex shrink-0 flex-col"
        style={{
          width: chartWidth,
          padding: `${padTop}px ${padX}px 0`,
        }}
      >
        {ready ? (
          <>
            <div className="flex">
              <RowLabels rows={shown.rows} width={labels} rowSize={rowSize} />
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
                  {shown.rows.map((row, index) =>
                    row.segments.map((segment) => {
                      const left = ((segment.from - from) / span) * plotWidth
                      const length =
                        ((segment.to - segment.from) / span) * plotWidth
                      const across = segmentSpan(
                        left,
                        length,
                        cells ? options.columnWidth : undefined,
                        cells
                      )
                      const top = index * rowSize + bar.offset
                      const place = textPlacement(
                        options.alignValue,
                        across.size
                      )
                      return (
                        <g key={`${row.key}:${segment.from}`}>
                          <rect
                            x={across.offset}
                            y={top}
                            width={across.size}
                            height={bar.size}
                            rx={2}
                            style={{
                              fill: segment.color,
                              fillOpacity: row.opacity,
                              stroke:
                                row.border > 0 ? segment.color : undefined,
                              strokeWidth: row.border,
                            }}
                          />
                          {showsText(mode, length, bar.size) ? (
                            <svg
                              x={across.offset}
                              y={top}
                              width={across.size}
                              height={bar.size}
                            >
                              <text
                                x={place.x}
                                y={bar.size / 2}
                                textAnchor={place.anchor}
                                dominantBaseline="central"
                                fontSize={12}
                                style={{
                                  fill:
                                    colors.get(
                                      `${row.opacity}|${segment.color}`
                                    ) ?? fallbackText,
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
                {hover && hovered ? (
                  <Tooltip
                    name={hovered.name}
                    segment={segmentAt(hovered.segments, hover.at)}
                    entries={hoveredEntries}
                    at={hover.at}
                    hovered={hover.row}
                    x={hover.x}
                    y={hover.y}
                    width={plotWidth}
                    height={plotHeight}
                    maxWidth={options.tooltipMaxWidth}
                    maxHeight={options.tooltipMaxHeight}
                  />
                ) : null}
              </div>
            </div>
            <div style={{ marginLeft: labels }}>
              <TimeLabels from={from} to={to} width={plotWidth} />
            </div>
          </>
        ) : null}
      </div>
      {legendShown && ready && legendMode === "table" ? (
        <LegendTable {...legendProps} />
      ) : null}
      {legendShown && ready && legendMode === "list" ? (
        <Legend {...legendProps} />
      ) : null}
    </div>
  )
}
