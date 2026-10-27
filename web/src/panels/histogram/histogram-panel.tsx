import { memo, useCallback, useMemo, useState, type MouseEvent } from "react"

import { formatValue } from "@/lib/units"

import type { PanelRenderer } from "../types"
import {
  countTicks,
  histogramModel,
  labelStride,
  stackedMax,
  type HistogramModel,
} from "./histogram-model"

const padTop = 8
const padLeft = 4
const padRight = 12
const xAxisHeight = 24
const legendHeight = 24
const tickSpacing = 28
const characterWidth = 7
const labelGap = 12
const tooltipOffset = 12
const minGapBucketWidth = 3

type Hover = { index: number; x: number; y: number }

function bucketText(
  model: HistogramModel,
  index: number,
  format: (edge: number) => string
): string {
  return `${format(model.edges[index])} - ${format(model.edges[index + 1])}`
}

const Bars = memo(function Bars({
  model,
  top,
  ticks,
  plotWidth,
  hovered,
}: {
  model: HistogramModel
  top: number
  ticks: number[]
  plotWidth: number
  hovered: number | null
}) {
  const buckets = model.edges.length - 1
  const bucketWidth = plotWidth / buckets
  const gap = bucketWidth >= minGapBucketWidth ? 1 : 0
  return (
    <>
      {ticks.map((tick) => (
        <line
          key={tick}
          x1={0}
          x2={plotWidth}
          y1={100 - (tick / top) * 100}
          y2={100 - (tick / top) * 100}
          vectorEffect="non-scaling-stroke"
          style={{ stroke: "var(--border)", strokeWidth: 1 }}
        />
      ))}
      {Array.from({ length: buckets }, (_, index) => {
        let base = 0
        return (
          <g
            key={index}
            style={{
              opacity: hovered !== null && hovered !== index ? 0.5 : 1,
            }}
          >
            {model.series.map((series) => {
              const count = series.counts[index]
              if (count === 0) return null
              base += count
              return (
                <rect
                  key={series.key}
                  x={index * bucketWidth + gap / 2}
                  width={bucketWidth - gap}
                  y={100 - (base / top) * 100}
                  height={(count / top) * 100}
                  style={{ fill: series.color }}
                />
              )
            })}
          </g>
        )
      })}
    </>
  )
})

function YLabels({
  ticks,
  top,
  width,
}: {
  ticks: number[]
  top: number
  width: number
}) {
  return (
    <div
      aria-hidden="true"
      className="relative shrink-0 text-xs text-muted-foreground tabular-nums"
      style={{ width }}
    >
      {ticks.map((tick) => (
        <span
          key={tick}
          className="absolute right-2 translate-y-1/2 leading-4"
          style={{ bottom: `${(tick / top) * 100}%` }}
        >
          {tick}
        </span>
      ))}
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

function Legend({ series }: { series: HistogramModel["series"] }) {
  return (
    <ul
      aria-label="Legend"
      className="m-0 flex max-h-[40%] shrink-0 list-none flex-wrap gap-x-4 gap-y-1 overflow-y-auto px-3 pt-1 pb-2 text-xs"
    >
      {series.map((item) => (
        <li key={item.key} className="flex min-w-0 items-center gap-1.5">
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0"
            style={{ backgroundColor: item.color }}
          />
          <span title={item.name} className="min-w-0 truncate">
            {item.name}
          </span>
        </li>
      ))}
    </ul>
  )
}

function Tooltip({
  model,
  hover,
  plotWidth,
  format,
}: {
  model: HistogramModel
  hover: Hover
  plotWidth: number
  format: (edge: number) => string
}) {
  const onRight = hover.x > plotWidth / 2
  const rows = model.series.filter(({ counts }) => counts[hover.index] > 0)
  const total = model.series.reduce(
    (sum, { counts }) => sum + counts[hover.index],
    0
  )
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.max(0, hover.y - tooltipOffset),
        ...(onRight
          ? { right: plotWidth - hover.x + tooltipOffset }
          : { left: hover.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 text-muted-foreground tabular-nums">
        {bucketText(model, hover.index, format)}
      </div>
      {model.series.length > 1
        ? rows.map((series) => (
            <div key={series.key} className="flex items-center gap-1.5">
              <span
                aria-hidden="true"
                className="size-2 shrink-0"
                style={{ backgroundColor: series.color }}
              />
              <span className="min-w-0 flex-1 truncate">{series.name}</span>
              <span className="tabular-nums">{series.counts[hover.index]}</span>
            </div>
          ))
        : null}
      <div className="flex justify-between gap-3 font-medium">
        <span>Total</span>
        <span className="tabular-nums">{total}</span>
      </div>
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

export const HistogramPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const [hover, setHover] = useState<Hover | null>(null)
  const model = useMemo(
    () => histogramModel(frames, fields, display),
    [frames, fields, display]
  )
  const buckets = Math.max(0, model.edges.length - 1)
  const multiple = model.series.length > 1
  const top = Math.max(1, stackedMax(model))
  const [measuredHeight, plotRef] = useMeasuredHeight()
  const estimatedHeight =
    height - padTop - xAxisHeight - (multiple ? legendHeight : 0)
  const plotHeight = measuredHeight ?? estimatedHeight
  const ticks = useMemo(
    () => countTicks(top, plotHeight / tickSpacing),
    [top, plotHeight]
  )
  const yTop = ticks[ticks.length - 1]
  const yAxisWidth = padLeft + String(yTop).length * characterWidth + labelGap
  const plotWidth = width - yAxisWidth - padRight
  const format = useMemo(
    () => (edge: number) => formatValue(edge, model.unit, model.decimals),
    [model.unit, model.decimals]
  )
  const labels = useMemo(() => model.edges.map(format), [model.edges, format])
  const ready = buckets > 0 && plotWidth > 0 && estimatedHeight > 0

  function onMove(event: MouseEvent<HTMLDivElement>) {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left
    const index = Math.min(
      buckets - 1,
      Math.max(0, Math.floor((x / box.width) * buckets))
    )
    setHover({ index, x, y: event.clientY - box.top })
  }

  return (
    <div
      data-buckets={buckets}
      data-series={model.series.length}
      className="flex flex-col"
      style={{ width, height }}
    >
      {ready ? (
        <>
          <div
            className="flex min-h-0 flex-1"
            style={{ paddingTop: padTop, paddingRight: padRight }}
          >
            <YLabels ticks={ticks} top={yTop} width={yAxisWidth} />
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
                  top={yTop}
                  ticks={ticks}
                  plotWidth={plotWidth}
                  hovered={hover?.index ?? null}
                />
              </svg>
              {hover ? (
                <Tooltip
                  model={model}
                  hover={hover}
                  plotWidth={plotWidth}
                  format={format}
                />
              ) : null}
            </div>
          </div>
          <XLabels labels={labels} plotWidth={plotWidth} offset={yAxisWidth} />
          {multiple ? <Legend series={model.series} /> : null}
        </>
      ) : null}
    </div>
  )
}

export default HistogramPanel
