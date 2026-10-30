import {
  memo,
  useCallback,
  useMemo,
  useState,
  type CSSProperties,
  type MouseEvent,
} from "react"

import { formatValue } from "@/lib/units"
import { cn } from "@/lib/utils"

import { useSeriesLegend } from "../legend/use-series-legend"
import type { PanelRenderer } from "../types"
import { scalePosition } from "../value-axis"
import {
  histogramEntries,
  histogramModel,
  histogramOptions,
  labelStride,
  legendEntries,
  tooltipSeries,
  valueTicks,
  type HistogramHover,
  type HistogramModel,
  type HistogramOptions,
} from "./histogram-model"

const padTop = 8
const padLeft = 4
const padRight = 12
const xAxisHeight = 24
const legendHeight = 28
const tickSpacing = 28
const characterWidth = 7
const labelGap = 12
const tooltipOffset = 12
const minGapBucketWidth = 3
const axisLabelThickness = 18

type Hover = HistogramHover & { x: number; y: number }

function bucketText(
  model: HistogramModel,
  index: number,
  format: (edge: number) => string
): string {
  return `${format(model.edges[index])} - ${format(model.edges[index + 1])}`
}

const Bars = memo(function Bars({
  model,
  min,
  max,
  ticks,
  plotWidth,
  hovered,
}: {
  model: HistogramModel
  min: number
  max: number
  ticks: number[]
  plotWidth: number
  hovered: number | null
}) {
  const buckets = model.edges.length - 1
  const bucketWidth = plotWidth / buckets
  const gap = bucketWidth >= minGapBucketWidth ? 1 : 0
  const { settings, color } = model.axis
  const position = (value: number) =>
    scalePosition(value, min, max, model.scale) * 100
  const borderX = settings.side === "end" ? plotWidth : 0
  return (
    <>
      {settings.grid
        ? ticks.map((tick) => (
            <line
              key={tick}
              x1={0}
              x2={plotWidth}
              y1={100 - position(tick)}
              y2={100 - position(tick)}
              vectorEffect="non-scaling-stroke"
              style={{ stroke: "var(--border)", strokeWidth: 1 }}
            />
          ))
        : null}
      {Array.from({ length: buckets }, (_, index) => (
        <g
          key={index}
          style={{
            opacity: hovered !== null && hovered !== index ? 0.5 : 1,
          }}
        >
          {model.series.map((series, seriesIndex) => {
            if (series.counts[index] === 0) return null
            const { start, end } = model.bars[seriesIndex][index]
            const low = position(start)
            const high = position(end)
            const style: CSSProperties = {
              fill: series.color,
              fillOpacity: series.fillOpacity,
              stroke: series.borderWidth > 0 ? series.color : "none",
              strokeWidth: series.borderWidth,
            }
            return (
              <rect
                key={series.key}
                data-series={series.key}
                x={index * bucketWidth + gap / 2}
                width={bucketWidth - gap}
                y={100 - high}
                height={Math.max(0, high - low)}
                vectorEffect="non-scaling-stroke"
                style={style}
              />
            )
          })}
        </g>
      ))}
      {settings.border && settings.side !== "hidden" ? (
        <line
          data-part="axis-border"
          x1={borderX}
          x2={borderX}
          y1={0}
          y2={100}
          vectorEffect="non-scaling-stroke"
          style={{ stroke: color ?? "var(--muted-foreground)", strokeWidth: 1 }}
        />
      ) : null}
    </>
  )
})

function YLabels({
  labels,
  min,
  max,
  model,
  width,
  label,
  color,
  atEnd,
}: {
  labels: { tick: number; text: string }[]
  min: number
  max: number
  model: HistogramModel
  width: number
  label: string | undefined
  color: string | null
  atEnd: boolean
}) {
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs tabular-nums"
      style={{ width, color: color ?? "var(--muted-foreground)" }}
    >
      {labels.map(({ tick, text }) => (
        <span
          key={tick}
          className={cn(
            "absolute translate-y-1/2 leading-4",
            atEnd ? "left-2" : "right-2"
          )}
          style={{
            bottom: `${scalePosition(tick, min, max, model.scale) * 100}%`,
          }}
        >
          {text}
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
  labels,
  plotWidth,
  offset,
}: {
  labels: string[]
  plotWidth: number
  offset: number
}) {
  const widest = Math.max(0, ...labels.map((label) => label.length))
  const stride = labelStride(
    labels.length,
    plotWidth,
    widest * characterWidth + labelGap
  )
  const shown: { index: number; label: string }[] = []
  labels.forEach((label, index) => {
    if (index % stride !== 0) return
    const half = (label.length * characterWidth) / 2
    const center = (index * plotWidth) / (labels.length - 1)
    if (center - half < -offset || center + half > plotWidth + padRight) return
    if (shown.at(-1)?.label === label) return
    shown.push({ index, label })
  })
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{ marginLeft: offset, width: plotWidth, height: xAxisHeight }}
    >
      {shown.map(({ index, label }) => (
        <span
          key={index}
          className="absolute top-2 -translate-x-1/2 leading-4 whitespace-nowrap"
          style={{ left: (index * plotWidth) / (labels.length - 1) }}
        >
          {label}
        </span>
      ))}
    </div>
  )
}

function Tooltip({
  model,
  hover,
  plotWidth,
  format,
  options,
}: {
  model: HistogramModel
  hover: Hover
  plotWidth: number
  format: (edge: number) => string
  options: HistogramOptions
}) {
  const onRight = hover.x > plotWidth / 2
  const { rows, total } = tooltipSeries(model, hover, options)
  const all = options.tooltip === "all"
  const listed = all && model.series.length < 2 ? [] : rows
  if (listed.length === 0 && !all) return null
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 overflow-hidden rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.max(0, hover.y - tooltipOffset),
        maxWidth: options.tooltipMaxWidth,
        maxHeight: options.tooltipMaxHeight,
        ...(onRight
          ? { right: plotWidth - hover.x + tooltipOffset }
          : { left: hover.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 text-muted-foreground tabular-nums">
        {bucketText(model, hover.index, format)}
      </div>
      {listed.map(({ index }) => {
        const series = model.series[index]
        return (
          <div key={series.key} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className="size-2 shrink-0"
              style={{ backgroundColor: series.color }}
            />
            <span className="min-w-0 flex-1 truncate">{series.name}</span>
            <span className="tabular-nums">{series.counts[hover.index]}</span>
          </div>
        )
      })}
      {all ? (
        <div className="flex justify-between gap-3 font-medium">
          <span>Total</span>
          <span className="tabular-nums">{total}</span>
        </div>
      ) : null}
    </div>
  )
}

function useMeasuredHeight(): [
  number | null,
  (node: HTMLDivElement | null) => void,
] {
  const [measured, setMeasured] = useState<number | null>(null)
  const ref = useCallback((node: HTMLDivElement | null) => {
    if (node === null) return
    const observer = new ResizeObserver(([entry]) =>
      setMeasured(entry.contentRect.height)
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  return [measured, ref]
}

const HistogramPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
  stateScope,
}) => {
  const [hover, setHover] = useState<Hover | null>(null)
  const options = useMemo(() => histogramOptions(display), [display])
  const entries = useMemo(
    () => histogramEntries(frames, fields, options.combine),
    [frames, fields, options.combine]
  )
  const legend = useSeriesLegend({
    stateKey: `${stateScope}:${panel.id}`,
    entries: useMemo(() => legendEntries(entries), [entries]),
    options,
    width,
    height,
  })
  const model = useMemo(
    () => histogramModel(entries, display, legend.hidden),
    [entries, display, legend.hidden]
  )
  const buckets = Math.max(0, model.edges.length - 1)
  const [measuredHeight, plotRef] = useMeasuredHeight()
  const estimatedHeight =
    height -
    padTop -
    xAxisHeight -
    (legend.placement === "bottom" ? legendHeight : 0)
  const plotHeight = measuredHeight ?? estimatedHeight
  const axis = useMemo(
    () => valueTicks(model, plotHeight / tickSpacing),
    [model, plotHeight]
  )
  const labels = useMemo(
    () =>
      axis.ticks.map((tick) => ({
        tick,
        text:
          model.stack === "percent"
            ? formatValue(tick, "percent", 0)
            : String(tick),
      })),
    [axis.ticks, model.stack]
  )
  const { settings } = model.axis
  const hiddenAxis = settings.side === "hidden"
  const atEnd = settings.side === "end"
  const labelsWidth =
    padLeft +
    Math.max(0, ...labels.map(({ text }) => text.length)) * characterWidth +
    labelGap
  const axisWidth = hiddenAxis
    ? padLeft
    : (settings.width ?? labelsWidth) +
      (settings.label ? axisLabelThickness : 0)
  const leftWidth = atEnd || hiddenAxis ? padLeft : axisWidth
  const rightWidth = atEnd ? axisWidth : 0
  const plotWidth = width - legend.width - leftWidth - rightWidth - padRight
  const format = useMemo(
    () => (edge: number) => formatValue(edge, model.unit, model.decimals),
    [model.unit, model.decimals]
  )
  const edgeLabels = useMemo(
    () => model.edges.map(format),
    [model.edges, format]
  )
  const ready = buckets > 0 && plotWidth > 0 && estimatedHeight > 0

  function onMove(event: MouseEvent<HTMLDivElement>) {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left
    const index = Math.min(
      buckets - 1,
      Math.max(0, Math.floor((x / box.width) * buckets))
    )
    const target = event.target as Element
    const series =
      target.closest("[data-series]")?.getAttribute("data-series") ?? null
    setHover({ index, series, x, y: event.clientY - box.top })
  }

  const hoverShown =
    hover && options.tooltip !== "hidden" && model.series.length > 0
      ? hover
      : null
  const yLabels = (
    <YLabels
      labels={labels}
      min={axis.min}
      max={axis.max}
      model={model}
      width={atEnd ? rightWidth : leftWidth}
      label={settings.label}
      color={model.axis.color}
      atEnd={atEnd}
    />
  )

  return (
    <div
      data-buckets={buckets}
      data-series={model.series.length}
      data-legend={legend.placement ?? "hidden"}
      className={cn(
        "flex",
        legend.placement === "right" ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      {ready ? (
        <>
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div
              className="flex min-h-0 flex-1"
              style={{ paddingTop: padTop, paddingRight: padRight }}
            >
              {!atEnd && !hiddenAxis ? (
                yLabels
              ) : (
                <div style={{ width: padLeft }} />
              )}
              <div
                ref={plotRef}
                className="relative min-w-0 flex-1"
                onMouseMove={onMove}
                onMouseLeave={() => setHover(null)}
              >
                <svg
                  role="img"
                  aria-label={`Histogram of ${model.total} values in ${buckets} buckets`}
                  viewBox={`0 0 ${plotWidth} 100`}
                  preserveAspectRatio="none"
                  className="size-full"
                >
                  <Bars
                    model={model}
                    min={axis.min}
                    max={axis.max}
                    ticks={axis.ticks}
                    plotWidth={plotWidth}
                    hovered={hover?.index ?? null}
                  />
                </svg>
                {hoverShown ? (
                  <Tooltip
                    model={model}
                    hover={hoverShown}
                    plotWidth={plotWidth}
                    format={format}
                    options={options}
                  />
                ) : null}
              </div>
              {atEnd ? yLabels : null}
            </div>
            <XLabels
              labels={edgeLabels}
              plotWidth={plotWidth}
              offset={leftWidth}
            />
          </div>
          {legend.node}
        </>
      ) : null}
    </div>
  )
}

export default HistogramPanel
