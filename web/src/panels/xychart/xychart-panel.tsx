import { useCallback, useMemo, useState } from "react"

import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { useSeriesLegend } from "../legend/use-series-legend"
import type { PanelRenderer } from "../types"
import { toPixelX, toPixelY, XyCanvas } from "./xy-canvas"
import {
  axisTicks,
  legendEntries,
  nearestPoint,
  pointSummary,
  pointsAtX,
  tooltipHits,
  xyData,
  xyModel,
  xyOptions,
  type XyAxis,
  type XyHit,
  type XyModel,
  type XyOptions,
  type XyTick,
} from "./xy-model"

const padTop = 8
const padLeft = 4
const padRight = 12
const xTickHeight = 24
const axisLabelThickness = 18
const characterWidth = 7
const minYLabelWidth = 28
const yTickSpacing = 40
const xTickSpacing = 88
const hoverRadius = 24
const tooltipOffset = 12
const markerPadding = 4
const minMarker = 12

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
  atEnd,
}: {
  ticks: XyTick[]
  model: XyModel
  width: number
  height: number
  atEnd: boolean
}) {
  const { label } = model.y.settings
  const tint = model.y.color ?? undefined
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{ width, height, color: tint }}
    >
      {ticks.map((tick) => (
        <span
          key={tick.value}
          className={cn(
            "absolute translate-y-[-50%] leading-4 whitespace-nowrap",
            atEnd ? "left-2" : "right-2"
          )}
          style={{ top: toPixelY(model, tick.value, height) }}
        >
          {tick.text}
        </span>
      ))}
      {label ? (
        <span
          data-part="axis-label"
          className="absolute top-0 h-full text-center leading-[18px] whitespace-nowrap"
          style={{
            writingMode: "vertical-rl",
            transform: atEnd ? undefined : "rotate(180deg)",
            [atEnd ? "right" : "left"]: 0,
          }}
        >
          {label}
        </span>
      ) : null}
    </div>
  )
}

function XLabels({
  ticks,
  model,
  width,
  offset,
  axis,
}: {
  ticks: XyTick[]
  model: XyModel
  width: number
  offset: number
  axis: XyAxis
}) {
  const { label } = axis.settings
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{
        marginLeft: offset,
        width,
        height: xTickHeight + (label ? axisLabelThickness : 0),
        color: axis.color ?? undefined,
      }}
    >
      {ticks.map((tick) => (
        <span
          key={tick.value}
          className="absolute top-1.5 -translate-x-1/2 leading-4 whitespace-nowrap"
          style={{ left: toPixelX(model, tick.value, width) }}
        >
          {tick.text}
        </span>
      ))}
      {label ? (
        <span
          data-part="axis-label"
          className="absolute inset-x-0 text-center leading-[18px] whitespace-nowrap"
          style={{ top: xTickHeight }}
        >
          {label}
        </span>
      ) : null}
    </div>
  )
}

function Tooltip({
  model,
  hits,
  at,
  width,
  height,
  options,
}: {
  model: XyModel
  hits: XyHit[]
  at: Hover
  width: number
  height: number
  options: XyOptions
}) {
  const onRight = at.x > width / 2
  const first = pointSummary(model, hits[0])
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 overflow-hidden rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(at.y + tooltipOffset, Math.max(0, height - 64)),
        maxWidth: options.tooltipMaxWidth,
        maxHeight: options.tooltipMaxHeight,
        ...(onRight
          ? { right: width - at.x + tooltipOffset }
          : { left: at.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 flex justify-between gap-3 text-muted-foreground tabular-nums">
        <span className="min-w-0 truncate">
          {model.series[hits[0].series].xName}
        </span>
        <span>{first.x}</span>
      </div>
      {hits.map((hit) => {
        const series = model.series[hit.series]
        const text = pointSummary(model, hit)
        return (
          <div key={series.key}>
            <div className="flex items-center gap-1.5">
              <span
                aria-hidden="true"
                className="size-2 shrink-0"
                style={{ backgroundColor: series.points[hit.index][2] }}
              />
              <span className="min-w-0 flex-1 truncate">{series.name}</span>
              <span className="font-medium tabular-nums">{text.y}</span>
            </div>
            {text.extras.map((extra) => (
              <div
                key={extra.name}
                className="flex justify-between gap-3 pl-3.5 text-muted-foreground tabular-nums"
              >
                <span className="min-w-0 truncate">{extra.name}</span>
                <span>{extra.text}</span>
              </div>
            ))}
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

const XychartPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
  stateScope,
}) => {
  const { resolved: theme } = useTheme()
  const [size, measure] = useMeasuredSize()
  const [hover, setHover] = useState<Hover | null>(null)
  const options = useMemo(() => xyOptions(display), [display])
  const data = useMemo(
    () => xyData(frames, fields, display),
    [frames, fields, display]
  )
  const legend = useSeriesLegend({
    stateKey: `${stateScope}:${panel.id}`,
    entries: useMemo(() => legendEntries(data.series), [data.series]),
    options,
    width,
    height,
  })
  const model = useMemo(
    () => xyModel(data, legend.hidden),
    [data, legend.hidden]
  )
  const { show, tooltip } = options
  const yHidden = model.y.settings.side === "hidden"
  const xHidden = model.x.settings.side === "hidden"
  const atEnd = model.y.settings.side === "end"

  const xAxisHeight = xHidden
    ? 0
    : xTickHeight + (model.x.settings.label ? axisLabelThickness : 0)
  const plotHeight = size ? size.height - xAxisHeight - padTop : 0
  const yTicks = useMemo(
    () =>
      axisTicks(
        model.y.range,
        model.y.config,
        Math.floor(plotHeight / yTickSpacing),
        model.y.scale
      ),
    [model.y, plotHeight]
  )
  const widest = Math.max(0, ...yTicks.map((tick) => tick.text.length))
  const autoWidth =
    Math.max(minYLabelWidth, widest * characterWidth + 12) +
    (model.y.settings.label ? axisLabelThickness : 0)
  const axisWidth = yHidden ? padLeft : (model.y.settings.width ?? autoWidth)
  const leftWidth = atEnd ? padLeft : axisWidth
  const rightWidth = (atEnd ? axisWidth : 0) + padRight
  const plotWidth = size ? size.width - leftWidth - rightWidth : 0
  const xTicks = useMemo(
    () =>
      axisTicks(
        model.x.range,
        model.x.config,
        Math.floor(plotWidth / xTickSpacing),
        model.x.scale
      ),
    [model.x, plotWidth]
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
    const found =
      tooltip === "single"
        ? [nearest]
        : pointsAtX(
            model,
            model.series[nearest.series].points[nearest.index][0]
          )
    return tooltipHits(model, found, options)
  }, [hover, tooltip, ready, model, plotWidth, plotHeight, options])

  const marked = hits[0]
  const markedPoint = marked
    ? model.series[marked.series].points[marked.index]
    : null
  const markerSize = markedPoint
    ? Math.max(minMarker, markedPoint[3] + markerPadding)
    : minMarker

  const yLabels = (
    <YLabels
      ticks={yTicks}
      model={model}
      width={atEnd ? axisWidth : leftWidth}
      height={plotHeight}
      atEnd={atEnd}
    />
  )

  return (
    <div
      data-series={model.series.length}
      data-show={show}
      data-legend={legend.placement ?? "hidden"}
      className={cn(
        "flex min-h-0 min-w-0",
        legend.placement === "right" ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      {data.reason ? (
        <Message>{data.reason}</Message>
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
                  {atEnd || yHidden ? (
                    <div style={{ width: padLeft }} />
                  ) : (
                    yLabels
                  )}
                  <div className="relative" style={{ width: plotWidth }}>
                    <XyCanvas
                      model={model}
                      width={plotWidth}
                      height={plotHeight}
                      theme={theme}
                      show={show}
                      xTicks={xTicks}
                      yTicks={yTicks}
                      label={`XY chart with ${model.series.length} series`}
                      onHover={setHover}
                    />
                    {markedPoint ? (
                      <span
                        aria-hidden="true"
                        className="pointer-events-none absolute -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-foreground"
                        style={{
                          width: markerSize,
                          height: markerSize,
                          left: toPixelX(model, markedPoint[0], plotWidth),
                          top: toPixelY(model, markedPoint[1], plotHeight),
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
                        options={options}
                      />
                    ) : null}
                  </div>
                  {atEnd && !yHidden ? yLabels : null}
                </div>
                {xHidden ? null : (
                  <XLabels
                    ticks={xTicks}
                    model={model}
                    width={plotWidth}
                    offset={leftWidth}
                    axis={model.x}
                  />
                )}
              </>
            ) : null}
          </div>
          {legend.node}
        </>
      )}
    </div>
  )
}

export default XychartPanel
