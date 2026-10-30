import { useMemo, useState } from "react"

import { formatDateTime } from "@/lib/time"
import { useTheme } from "@/lib/theme"

import { CursorLine, useCursorLine } from "../cursor-line"
import { exemplarPoints } from "../timeseries/exemplars"
import type { PanelRenderer } from "../types"
import { timeTicks } from "@/lib/time-ticks"
import { HeatmapCanvas, type Hover } from "./heatmap-canvas"
import {
  cellCenter,
  cellText,
  columnAt,
  formatCounts,
  heatmapLook,
  heatmapModel,
  type HeatmapCell,
  type HeatmapLook,
  legendDescription,
  legendScale,
  type HeatmapModel,
} from "./heatmap-model"
import {
  axisLook,
  columnEntries,
  exemplarLook,
  exemplarMarks,
  histogramBars,
  markAt,
  scaleMarker,
  tooltipLook,
  type AxisLook,
  type ExemplarMark,
  type TooltipLook,
} from "./heatmap-overlay"

const padX = 12
const padTop = 6
const xAxisHeight = 20
const legendHeight = 24
const legendGap = 8
const rowLabelHeight = 16
const tickSpacing = 72
const tickEdgeGap = 20
const characterWidth = 7
const tooltipOffset = 12
const axisTitleWidth = 16
const histogramWidth = 96
const histogramHeight = 40
const diamondRadius = 3.5

function autoAxisWidth(model: HeatmapModel): number {
  const longest = Math.max(0, ...model.rows.map((row) => row.label.length))
  return Math.min(96, Math.max(32, longest * characterWidth + 8))
}

function RowLabels({
  model,
  side,
  width,
  height,
}: {
  model: HeatmapModel
  side: "left" | "right"
  width: number
  height: number
}) {
  const rowHeight = height / model.rows.length
  const stride = Math.max(1, Math.ceil(rowLabelHeight / rowHeight))
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{ width, height }}
    >
      {model.rows.map((row, index) =>
        index % stride === 0 ? (
          <span
            key={index}
            className={`absolute truncate leading-4 ${side === "left" ? "right-2" : "left-2"}`}
            style={{
              top: height - (index + 0.5) * rowHeight - rowLabelHeight / 2,
              maxWidth: width - 8,
            }}
          >
            {row.label}
          </span>
        ) : null
      )}
    </div>
  )
}

function AxisTitle({
  text,
  side,
  height,
}: {
  text: string
  side: "left" | "right"
  height: number
}) {
  return (
    <div
      className="flex shrink-0 items-center justify-center text-xs text-muted-foreground"
      style={{ width: axisTitleWidth, height }}
    >
      <span
        className="truncate whitespace-nowrap"
        style={{
          writingMode: "vertical-rl",
          transform: side === "left" ? "rotate(180deg)" : undefined,
          maxHeight: height,
        }}
      >
        {text}
      </span>
    </div>
  )
}

function TimeLabels({ xs, width }: { xs: number[]; width: number }) {
  const ticks = timeTicks(xs, Math.floor(width / tickSpacing)).filter(
    (tick) => {
      const center = ((tick.index + 0.5) * width) / xs.length
      return center >= tickEdgeGap && center <= width - tickEdgeGap
    }
  )
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
          style={{ left: ((tick.index + 0.5) * width) / xs.length }}
        >
          {tick.label}
        </span>
      ))}
    </div>
  )
}

function Legend({
  look,
  format,
}: {
  look: HeatmapLook
  format: (value: number) => string
}) {
  return (
    <div
      role="img"
      aria-label={legendDescription(look, format)}
      className="flex items-center justify-center gap-2 text-xs text-muted-foreground tabular-nums"
      style={{ height: legendHeight, marginTop: legendGap }}
    >
      <span aria-hidden="true">{format(look.labelLow)}</span>
      <span aria-hidden="true" className="flex gap-px">
        {legendScale(look).map((step, index) => (
          <span
            key={index}
            className="block h-3 w-6"
            style={{
              backgroundColor: step.paint.color,
              opacity: step.paint.alpha,
            }}
          />
        ))}
      </span>
      <span aria-hidden="true">{format(look.labelHigh)}</span>
    </div>
  )
}

function Exemplars({
  marks,
  color,
  width,
  height,
}: {
  marks: ExemplarMark[]
  color: string
  width: number
  height: number
}) {
  return (
    <svg
      aria-hidden="true"
      className="pointer-events-none absolute inset-0"
      width={width}
      height={height}
    >
      {marks.map((mark, index) => (
        <polygon
          key={index}
          points={`${mark.x},${mark.y - diamondRadius} ${mark.x + diamondRadius},${mark.y} ${mark.x},${mark.y + diamondRadius} ${mark.x - diamondRadius},${mark.y}`}
          style={{ fill: color, stroke: "var(--background)", strokeWidth: 1 }}
        />
      ))}
    </svg>
  )
}

function ColumnHistogram({
  model,
  look,
  hover,
}: {
  model: HeatmapModel
  look: HeatmapLook
  hover: Hover
}) {
  const bars = histogramBars(model, look, hover.col)
  if (bars.length === 0) return null
  const barHeight = histogramHeight / bars.length
  return (
    <svg
      aria-hidden="true"
      className="mt-1.5"
      width={histogramWidth}
      height={histogramHeight}
    >
      {bars.map((bar, index) => (
        <rect
          key={bar.row}
          x={0}
          y={index * barHeight}
          width={Math.max(bar.share > 0 ? 1 : 0, bar.share * histogramWidth)}
          height={Math.max(1, barHeight - (barHeight > 3 ? 1 : 0))}
          style={{
            fill:
              bar.row === hover.row
                ? "var(--foreground)"
                : "var(--muted-foreground)",
          }}
        />
      ))}
    </svg>
  )
}

function ScaleMarker({ look, value }: { look: HeatmapLook; value: number }) {
  return (
    <div className="relative mt-1.5 flex gap-px" aria-hidden="true">
      {legendScale(look).map((step, index) => (
        <span
          key={index}
          className="block h-2 flex-1"
          style={{
            backgroundColor: step.paint.color,
            opacity: step.paint.alpha,
          }}
        />
      ))}
      <span
        className="absolute -top-0.5 -bottom-0.5 w-0.5 bg-foreground"
        style={{ left: `${scaleMarker(look, value) * 100}%` }}
      />
    </div>
  )
}

function Tooltip({
  model,
  look,
  tooltip,
  hover,
  exemplar,
  width,
  height,
  format,
}: {
  model: HeatmapModel
  look: HeatmapLook
  tooltip: TooltipLook
  hover: Hover
  exemplar: ExemplarMark | null
  width: number
  height: number
  format: (value: number) => string
}) {
  const value = model.cells[hover.row][hover.col]
  const onRight = hover.x > width / 2
  const listed =
    tooltip.mode === "all"
      ? columnEntries(model, look, hover.col, hover.row)
      : null
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 overflow-hidden rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(hover.y + tooltipOffset, Math.max(0, height - 64)),
        maxWidth: tooltip.maxWidth,
        maxHeight: tooltip.maxHeight,
        ...(onRight
          ? { right: width - hover.x + tooltipOffset }
          : { left: hover.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 text-muted-foreground tabular-nums">
        {formatDateTime(model.xs[hover.col])}
      </div>
      {listed === null ? (
        <div className="flex justify-between gap-3">
          <span className="min-w-0 truncate">
            {model.rows[hover.row].range}
          </span>
          <span className="font-medium tabular-nums">
            {value === null ? "-" : format(value)}
          </span>
        </div>
      ) : (
        <>
          {listed.entries.map((entry) => (
            <div
              key={entry.row}
              className={`flex justify-between gap-3 ${entry.row === hover.row ? "font-medium" : "text-muted-foreground"}`}
            >
              <span className="min-w-0 truncate">{entry.range}</span>
              <span className="tabular-nums">{format(entry.value)}</span>
            </div>
          ))}
          {listed.more > 0 ? (
            <div className="text-muted-foreground">{listed.more} more</div>
          ) : null}
        </>
      )}
      {tooltip.colorScale && value !== null && value > 0 ? (
        <ScaleMarker look={look} value={value} />
      ) : null}
      {tooltip.histogram ? (
        <ColumnHistogram model={model} look={look} hover={hover} />
      ) : null}
      {exemplar ? (
        <div className="mt-1.5 border-t border-border pt-1.5">
          <div className="font-medium">Exemplar</div>
          <div className="flex justify-between gap-3">
            <span className="text-muted-foreground">Value</span>
            <span className="tabular-nums">{exemplar.value}</span>
          </div>
          {exemplar.labels.map(([name, text]) => (
            <div key={name} className="flex justify-between gap-3">
              <span className="text-muted-foreground">{name}</span>
              <span className="min-w-0 truncate">{text}</span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function labelColumnWidth(axis: AxisLook, model: HeatmapModel): number {
  if (axis.side === "hidden") return 0
  return axis.width ?? autoAxisWidth(model)
}

const HeatmapPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
  cursor,
  exemplars,
}) => {
  const { resolved: theme } = useTheme()
  const [hover, setHover] = useState<Hover | null>(null)
  const [focus, setFocus] = useState<{
    model: HeatmapModel
    cell: HeatmapCell | null
  } | null>(null)
  const model = useMemo(
    () => heatmapModel(frames, fields, display, panel.queries),
    [frames, fields, display, panel.queries]
  )
  const look = useMemo(() => heatmapLook(display, model), [display, model])
  const axis = useMemo(() => axisLook(display), [display])
  const tooltip = useMemo(() => tooltipLook(display), [display])
  const exemplarStyle = useMemo(() => exemplarLook(display), [display])
  const format = useMemo(
    () => formatCounts([model.min, model.max], look.unit, look.decimals),
    [model.min, model.max, look.unit, look.decimals]
  )
  const legendShown = look.legend && model.max > 0
  const rows = model.rows.length
  const columns = model.xs.length
  const labelWidth = labelColumnWidth(axis, model)
  const titleWidth =
    axis.side !== "hidden" && axis.label !== undefined ? axisTitleWidth : 0
  const sideWidth = labelWidth + titleWidth
  const leftWidth = axis.side === "left" ? sideWidth : 0
  const rightWidth = axis.side === "right" ? sideWidth : 0
  const plotWidth = width - 2 * padX - sideWidth
  const plotHeight =
    height - padTop - xAxisHeight - (legendShown ? legendHeight + legendGap : 0)
  const points = useMemo(
    () =>
      exemplarStyle.shown
        ? exemplarPoints(exemplars ?? [], [], [-Infinity, Infinity])
        : [],
    [exemplarStyle.shown, exemplars]
  )
  const marks = useMemo(
    () =>
      plotWidth > 0 && plotHeight > 0
        ? exemplarMarks(model, points, plotWidth, plotHeight)
        : [],
    [model, points, plotWidth, plotHeight]
  )
  const focused = focus !== null && focus.model === model ? focus.cell : null
  const shown =
    hover ??
    (focused === null
      ? null
      : { ...focused, ...cellCenter(model, focused, plotWidth, plotHeight) })
  const ready = rows > 0 && columns > 0 && plotWidth > 0 && plotHeight > 0
  const { lineRef, move } = useCursorLine(cursor, (x) => {
    const col = columnAt(model.xs, x)
    return col === null ? null : ((col + 0.5) * plotWidth) / columns
  })
  const labels = (side: "left" | "right") => (
    <>
      {axis.label !== undefined && side === "left" ? (
        <AxisTitle text={axis.label} side={side} height={plotHeight} />
      ) : null}
      <RowLabels
        model={model}
        side={side}
        width={labelWidth}
        height={plotHeight}
      />
      {axis.label !== undefined && side === "right" ? (
        <AxisTitle text={axis.label} side={side} height={plotHeight} />
      ) : null}
    </>
  )
  return (
    <div
      data-rows={rows}
      data-columns={columns}
      data-mode={model.mode}
      className="flex flex-col"
      style={{ width, height, padding: `${padTop}px ${padX}px 0` }}
    >
      {ready ? (
        <>
          <div className="flex">
            {axis.side === "left" ? labels("left") : null}
            <div className="relative" style={{ width: plotWidth }}>
              <HeatmapCanvas
                model={model}
                look={look}
                format={format}
                width={plotWidth}
                height={plotHeight}
                theme={theme}
                label={`Heatmap with ${rows} rows over ${columns} time steps; highest count ${format(model.max)}`}
                focused={focused}
                onHover={setHover}
                onCursorMove={move}
                onFocusChange={(cell) => setFocus({ model, cell })}
              />
              <CursorLine lineRef={lineRef} height={plotHeight} />
              {marks.length > 0 ? (
                <Exemplars
                  marks={marks}
                  color={exemplarStyle.color}
                  width={plotWidth}
                  height={plotHeight}
                />
              ) : null}
              <div aria-live="polite" className="sr-only">
                {focused === null ? "" : cellText(model, focused, format)}
              </div>
              {tooltip.mode !== "hidden" &&
              shown &&
              shown.row < rows &&
              shown.col < columns ? (
                <Tooltip
                  model={model}
                  look={look}
                  tooltip={tooltip}
                  hover={shown}
                  exemplar={hover ? markAt(marks, hover.x, hover.y) : null}
                  width={plotWidth}
                  height={plotHeight}
                  format={format}
                />
              ) : null}
            </div>
            {axis.side === "right" ? labels("right") : null}
          </div>
          <div style={{ marginLeft: leftWidth, marginRight: rightWidth }}>
            <TimeLabels xs={model.xs} width={plotWidth} />
          </div>
          {legendShown ? <Legend look={look} format={format} /> : null}
        </>
      ) : null}
    </div>
  )
}

export default HeatmapPanel
