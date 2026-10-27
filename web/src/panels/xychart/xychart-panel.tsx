import { useCallback, useMemo, useState } from "react"

import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import type { PanelRenderer } from "../types"
import { toPixelX, toPixelY, XyCanvas } from "./xy-canvas"
import {
  axisTicks,
  legendOf,
  nearestPoint,
  pointSizeOf,
  pointSummary,
  pointsAtX,
  showOf,
  tooltipOf,
  xyModel,
  type XyHit,
  type XyModel,
  type XyTick,
} from "./xy-model"

const padTop = 8
const padRight = 12
const xAxisHeight = 24
const characterWidth = 7
const minYLabelWidth = 28
const yTickSpacing = 40
const xTickSpacing = 88
const hoverRadius = 24
const tooltipOffset = 12

type Size = { width: number; height: number }
type Hover = { x: number; y: number }

function useMeasuredSize(): [
  Size | null,
  (node: HTMLDivElement | null) => void,
] {
  const [size, setSize] = useState<Size | null>(null)
  const ref = useCallback((node: HTMLDivElement | null) => {
    if (node === null) return
    const observer = new ResizeObserver(([entry]) =>
      setSize({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      })
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  return [size, ref]
}

function YLabels({
  ticks,
  model,
  width,
  height,
}: {
  ticks: XyTick[]
  model: XyModel
  width: number
  height: number
}) {
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{ width, height }}
    >
      {ticks.map((tick) => (
        <span
          key={tick.value}
          className="absolute right-2 translate-y-[-50%] leading-4 whitespace-nowrap"
          style={{ top: toPixelY(tick.value, model.y, height) }}
        >
          {tick.text}
        </span>
      ))}
    </div>
  )
}

function XLabels({
  ticks,
  model,
  width,
  offset,
}: {
  ticks: XyTick[]
  model: XyModel
  width: number
  offset: number
}) {
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{ marginLeft: offset, width, height: xAxisHeight }}
    >
      {ticks.map((tick) => (
        <span
          key={tick.value}
          className="absolute top-1.5 -translate-x-1/2 leading-4 whitespace-nowrap"
          style={{ left: toPixelX(tick.value, model.x, width) }}
        >
          {tick.text}
        </span>
      ))}
    </div>
  )
}

function Legend({
  model,
  placement,
}: {
  model: XyModel
  placement: "bottom" | "right"
}) {
  const bottom = placement === "bottom"
  return (
    <ul
      aria-label="Legend"
      data-placement={placement}
      className={cn(
        "m-0 flex list-none overflow-y-auto p-0 text-xs",
        bottom
          ? "max-h-[40%] shrink-0 flex-wrap gap-x-4 gap-y-1 px-3 pt-1 pb-2"
          : "w-[clamp(140px,35%,260px)] shrink-0 flex-col gap-1 py-2 pr-3"
      )}
    >
      {model.series.map((series) => (
        <li key={series.key} className="flex min-w-0 items-center gap-1.5">
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0"
            style={{ backgroundColor: series.color }}
          />
          <span title={series.name} className="min-w-0 truncate">
            {series.name}
          </span>
        </li>
      ))}
    </ul>
  )
}

function Tooltip({
  model,
  hits,
  at,
  width,
  height,
}: {
  model: XyModel
  hits: XyHit[]
  at: Hover
  width: number
  height: number
}) {
  const onRight = at.x > width / 2
  const first = pointSummary(model, hits[0])
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(at.y + tooltipOffset, Math.max(0, height - 64)),
        ...(onRight
          ? { right: width - at.x + tooltipOffset }
          : { left: at.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 flex justify-between gap-3 text-muted-foreground tabular-nums">
        <span className="min-w-0 truncate">{model.xAxis?.name}</span>
        <span>{first.x}</span>
      </div>
      {hits.map((hit) => {
        const series = model.series[hit.series]
        const text = pointSummary(model, hit)
        return (
          <div key={series.key} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className="size-2 shrink-0"
              style={{ backgroundColor: series.points[hit.index][2] }}
            />
            <span className="min-w-0 flex-1 truncate">{series.name}</span>
            <span className="font-medium tabular-nums">{text.y}</span>
          </div>
        )
      })}
    </div>
  )
}

function Message({ children }: { children: string }) {
  return (
    <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
      {children}
    </div>
  )
}

export const XychartPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const { resolved: theme } = useTheme()
  const [size, measure] = useMeasuredSize()
  const [hover, setHover] = useState<Hover | null>(null)
  const model = useMemo(
    () => xyModel(frames, fields, display),
    [frames, fields, display]
  )
  const show = showOf(display)
  const legend = legendOf(display)
  const tooltip = tooltipOf(display)
  const pointSize = pointSizeOf(display)
  const showLegend = legend !== "hidden" && model.series.length > 1

  const widest = (ticks: XyTick[]) =>
    Math.max(0, ...ticks.map((tick) => tick.text.length))
  const plotHeight = size ? size.height - xAxisHeight : 0
  const yTicks = useMemo(
    () =>
      axisTicks(
        model.y,
        model.yAxis?.config,
        Math.floor(plotHeight / yTickSpacing)
      ),
    [model.y, model.yAxis, plotHeight]
  )
  const labelWidth = Math.max(
    minYLabelWidth,
    widest(yTicks) * characterWidth + 12
  )
  const plotWidth = size ? size.width - labelWidth - padRight : 0
  const xTicks = useMemo(
    () =>
      axisTicks(
        model.x,
        model.xAxis?.config,
        Math.floor(plotWidth / xTickSpacing)
      ),
    [model.x, model.xAxis, plotWidth]
  )
  const ready = size !== null && plotWidth > 0 && plotHeight > 0

  const hits = useMemo<XyHit[]>(() => {
    if (!hover || tooltip === "hidden" || !ready) return []
    const nearest = nearestPoint(model, hover.x, hover.y, {
      width: plotWidth,
      height: plotHeight,
      radius: hoverRadius,
    })
    if (!nearest) return []
    if (tooltip === "single") return [nearest]
    const x = model.series[nearest.series].points[nearest.index][0]
    return pointsAtX(model, x)
  }, [hover, tooltip, ready, model, plotWidth, plotHeight])

  const marked = hits[0]
  const markedPoint = marked
    ? model.series[marked.series].points[marked.index]
    : null

  return (
    <div
      data-series={model.series.length}
      data-show={show}
      data-legend={legend}
      className={cn(
        "flex min-h-0 min-w-0",
        legend === "right" ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      {model.reason ? (
        <Message>{model.reason}</Message>
      ) : (
        <>
          <div
            ref={measure}
            className="flex min-h-0 min-w-0 flex-1 flex-col"
            style={{ paddingTop: padTop }}
          >
            {ready ? (
              <>
                <div className="flex">
                  <YLabels
                    ticks={yTicks}
                    model={model}
                    width={labelWidth}
                    height={plotHeight}
                  />
                  <div className="relative" style={{ width: plotWidth }}>
                    <XyCanvas
                      model={model}
                      width={plotWidth}
                      height={plotHeight}
                      theme={theme}
                      show={show}
                      pointSize={pointSize}
                      xTicks={xTicks}
                      yTicks={yTicks}
                      label={`XY chart with ${model.series.length} series`}
                      onHover={setHover}
                    />
                    {markedPoint ? (
                      <span
                        aria-hidden="true"
                        className="pointer-events-none absolute size-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-foreground"
                        style={{
                          left: toPixelX(markedPoint[0], model.x, plotWidth),
                          top: toPixelY(markedPoint[1], model.y, plotHeight),
                        }}
                      />
                    ) : null}
                    {hover && hits.length > 0 ? (
                      <Tooltip
                        model={model}
                        hits={hits}
                        at={hover}
                        width={plotWidth}
                        height={plotHeight}
                      />
                    ) : null}
                  </div>
                </div>
                <XLabels
                  ticks={xTicks}
                  model={model}
                  width={plotWidth}
                  offset={labelWidth}
                />
              </>
            ) : null}
          </div>
          {showLegend ? (
            <Legend
              model={model}
              placement={legend === "right" ? "right" : "bottom"}
            />
          ) : null}
        </>
      )}
    </div>
  )
}

export default XychartPanel
