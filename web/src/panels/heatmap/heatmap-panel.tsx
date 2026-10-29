import { useMemo, useState } from "react"

import { formatDateTime } from "@/lib/time"
import { useTheme } from "@/lib/theme"

import { CursorLine, useCursorLine } from "../cursor-line"
import type { PanelRenderer } from "../types"
import { timeTicks } from "@/lib/time-ticks"
import { HeatmapCanvas, type Hover } from "./heatmap-canvas"
import {
  cellCenter,
  cellText,
  columnAt,
  formatCounts,
  heatmapModel,
  type HeatmapCell,
  legendScale,
  type HeatmapModel,
} from "./heatmap-model"

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

function axisWidth(model: HeatmapModel): number {
  const longest = Math.max(0, ...model.rows.map((row) => row.label.length))
  return Math.min(96, Math.max(32, longest * characterWidth + 8))
}

function RowLabels({
  model,
  width,
  height,
}: {
  model: HeatmapModel
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
            className="absolute right-2 truncate leading-4"
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
  min,
  max,
  format,
}: {
  min: number
  max: number
  format: (value: number) => string
}) {
  return (
    <div
      className="flex items-center justify-center gap-2 text-xs text-muted-foreground tabular-nums"
      style={{ height: legendHeight, marginTop: legendGap }}
    >
      <span>{format(min)}</span>
      <span aria-hidden="true" className="flex gap-px">
        {legendScale(min, max).map((step, index) => (
          <span
            key={index}
            className="block h-3 w-6"
            style={{
              backgroundColor: "var(--series-1)",
              opacity: step.opacity,
            }}
          />
        ))}
      </span>
      <span>{format(max)}</span>
    </div>
  )
}

function Tooltip({
  model,
  hover,
  width,
  height,
  format,
}: {
  model: HeatmapModel
  hover: Hover
  width: number
  height: number
  format: (value: number) => string
}) {
  const value = model.cells[hover.row][hover.col]
  const onRight = hover.x > width / 2
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(hover.y + tooltipOffset, Math.max(0, height - 64)),
        ...(onRight
          ? { right: width - hover.x + tooltipOffset }
          : { left: hover.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 text-muted-foreground tabular-nums">
        {formatDateTime(model.xs[hover.col])}
      </div>
      <div className="flex justify-between gap-3">
        <span className="min-w-0 truncate">{model.rows[hover.row].range}</span>
        <span className="font-medium tabular-nums">
          {value === null ? "-" : format(value)}
        </span>
      </div>
    </div>
  )
}

const HeatmapPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
  cursor,
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
  const format = useMemo(
    () => formatCounts([model.min, model.max]),
    [model.min, model.max]
  )
  const rows = model.rows.length
  const columns = model.xs.length
  const labelWidth = axisWidth(model)
  const plotWidth = width - 2 * padX - labelWidth
  const plotHeight = height - padTop - xAxisHeight - legendHeight - legendGap
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
            <RowLabels model={model} width={labelWidth} height={plotHeight} />
            <div className="relative" style={{ width: plotWidth }}>
              <HeatmapCanvas
                model={model}
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
              <div aria-live="polite" className="sr-only">
                {focused === null ? "" : cellText(model, focused, format)}
              </div>
              {shown && shown.row < rows && shown.col < columns ? (
                <Tooltip
                  model={model}
                  hover={shown}
                  width={plotWidth}
                  height={plotHeight}
                  format={format}
                />
              ) : null}
            </div>
          </div>
          <div style={{ marginLeft: labelWidth }}>
            <TimeLabels xs={model.xs} width={plotWidth} />
          </div>
          {model.max > 0 ? (
            <Legend min={model.min} max={model.max} format={format} />
          ) : null}
        </>
      ) : null}
    </div>
  )
}

export default HeatmapPanel
