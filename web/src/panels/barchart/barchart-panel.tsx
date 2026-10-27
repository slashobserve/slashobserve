import {
  memo,
  useCallback,
  useMemo,
  useState,
  type CSSProperties,
  type MouseEvent,
} from "react"

import { axisDecimals, formatValue } from "@/lib/units"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { resolveColor } from "../colors"
import { cssColorToRgb, readableText } from "../contrast"
import type { PanelRenderer } from "../types"
import {
  axisTicks,
  barRect,
  barchartModel,
  hitTest,
  labelHeight,
  labelSpot,
  labelStride,
  legendOf,
  shareOf,
  showValuesOf,
  tooltipOf,
  type BarSeries,
  type BarchartLegend,
  type BarchartModel,
  type BarchartTooltip,
  type Hit,
  type LabelSpot,
  type Layout,
  type Rect,
} from "./barchart-model"

const padTop = 8
const padRight = 12
const axisThickness = 24
const characterWidth = 7
const axisGap = 16
const tickSpacing = 32
const horizontalTickSpacing = 72
const maxLabelWidth = 120
const strideLabelWidth = 80
const tooltipOffset = 12
const sidebarShare = 0.4
const edgeGap = 4

type Size = { width: number; height: number }
type Pointer = Hit & { x: number; y: number }

function useMeasuredSize(): [
  Size | null,
  (node: HTMLDivElement | null) => void,
] {
  const [measured, setMeasured] = useState<Size | null>(null)
  const ref = useCallback((node: HTMLDivElement | null) => {
    if (node === null) return
    const observer = new ResizeObserver(([entry]) =>
      setMeasured({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      })
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  return [measured, ref]
}

function textWidth(text: string): number {
  return text.length * characterWidth
}

const Legend = memo(function Legend({
  series,
  placement,
}: {
  series: BarSeries[]
  placement: Exclude<BarchartLegend, "hidden">
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
          : "w-[clamp(120px,25%,220px)] shrink-0 flex-col gap-1 py-2 pr-3"
      )}
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
})

const Bars = memo(function Bars({
  model,
  layout,
  ticks,
  hovered,
}: {
  model: BarchartModel
  layout: Layout
  ticks: number[]
  hovered: Hit | null
}) {
  const vertical = layout.orientation === "vertical"
  const { width, height } = layout.plot
  const position = (value: number) => {
    const share = (value - layout.min) / (layout.max - layout.min)
    return vertical ? height - share * height : share * width
  }
  const zero = position(0)
  return (
    <>
      {ticks
        .filter((tick) => tick !== 0)
        .map((tick) => (
          <line
            key={tick}
            x1={vertical ? 0 : position(tick)}
            x2={vertical ? width : position(tick)}
            y1={vertical ? position(tick) : 0}
            y2={vertical ? position(tick) : height}
            style={{ stroke: "var(--border)", strokeWidth: 1 }}
          />
        ))}
      {model.bars.map((row, category) =>
        row.map((bar, index) => {
          if (bar.value === null) return null
          const series = model.series[index]
          const rect = barRect(layout, category, series.slot, bar)
          const outlined =
            hovered?.category === category && hovered.bar === index
          return (
            <rect
              key={`${category}:${series.key}`}
              data-series={series.key}
              {...rect}
              style={{
                fill: bar.color,
                stroke: outlined ? "var(--foreground)" : "none",
                strokeWidth: 1,
              }}
            />
          )
        })
      )}
      <line
        data-part="baseline"
        x1={vertical ? 0 : zero}
        x2={vertical ? width : zero}
        y1={vertical ? zero : 0}
        y2={vertical ? zero : height}
        style={{ stroke: "var(--border)", strokeWidth: 1 }}
      />
    </>
  )
})

function valueLabelStyle(
  rect: Rect,
  orientation: Layout["orientation"],
  plot: Size,
  spot: Exclude<LabelSpot, null>,
  negative: boolean
): CSSProperties {
  const inside = spot === "inside"
  if (orientation === "vertical") {
    const top = inside
      ? negative
        ? rect.y + rect.height - labelHeight - 2
        : rect.y + 2
      : negative
        ? rect.y + rect.height
        : rect.y - labelHeight
    return { left: rect.x + rect.width / 2, top, transform: "translateX(-50%)" }
  }
  const top = rect.y + rect.height / 2
  const transform = "translateY(-50%)"
  if (inside) {
    return negative
      ? { left: rect.x + 4, top, transform }
      : { right: plot.width - (rect.x + rect.width) + 4, top, transform }
  }
  return negative
    ? { right: plot.width - rect.x + 4, top, transform }
    : { left: rect.x + rect.width + 4, top, transform }
}

function ValueLabels({
  model,
  layout,
  mode,
  textColors,
}: {
  model: BarchartModel
  layout: Layout
  mode: ReturnType<typeof showValuesOf>
  textColors: Map<string, string | undefined>
}) {
  if (mode === "never") return null
  return (
    <>
      {model.bars.map((row, category) =>
        row.map((bar, index) => {
          if (bar.value === null) return null
          const series = model.series[index]
          const rect = barRect(layout, category, series.slot, bar)
          const negative = bar.end < bar.start
          const spot = labelSpot(
            mode,
            rect,
            layout.orientation,
            layout.plot,
            textWidth(bar.shown.text),
            { stacked: series.stack !== "none", negative }
          )
          if (spot === null) return null
          return (
            <span
              key={`${category}:${series.key}`}
              aria-hidden="true"
              className="pointer-events-none absolute text-xs leading-4 whitespace-nowrap tabular-nums"
              style={{
                ...valueLabelStyle(
                  rect,
                  layout.orientation,
                  layout.plot,
                  spot,
                  negative
                ),
                color:
                  spot === "inside" ? textColors.get(bar.color) : undefined,
              }}
            >
              {bar.shown.text}
            </span>
          )
        })
      )}
    </>
  )
}

function CategoryLabels({
  model,
  layout,
  labelWidth,
  gutter,
}: {
  model: BarchartModel
  layout: Layout
  labelWidth: number
  gutter: number
}) {
  const vertical = layout.orientation === "vertical"
  const across = vertical ? layout.plot.width : layout.plot.height
  const slot = across / layout.count
  const widest = Math.max(
    0,
    ...model.categories.map((category) => textWidth(category.label))
  )
  const stride = labelStride(
    layout.count,
    across,
    vertical ? Math.min(widest, strideLabelWidth) + 8 : labelHeight
  )
  return (
    <>
      {model.categories.map((category, index) => {
        if (index % stride !== 0) return null
        const center = index * slot + slot / 2
        const half = Math.min(
          (slot * stride) / 2,
          center + gutter - edgeGap,
          layout.plot.width - center + padRight - edgeGap
        )
        return (
          <span
            key={category.key}
            title={category.title}
            aria-hidden="true"
            className={cn(
              "absolute truncate text-xs leading-4 text-muted-foreground",
              vertical ? "top-full mt-2 text-center" : "pr-2 text-right"
            )}
            style={
              vertical
                ? {
                    left: center - half + edgeGap / 2,
                    width: half * 2 - edgeGap,
                  }
                : {
                    left: -labelWidth,
                    width: labelWidth,
                    top: center - labelHeight / 2,
                  }
            }
          >
            {category.label}
          </span>
        )
      })}
    </>
  )
}

function ValueAxis({
  layout,
  labels,
}: {
  layout: Layout
  labels: { tick: number; text: string }[]
}) {
  const vertical = layout.orientation === "vertical"
  const { width, height } = layout.plot
  return (
    <>
      {labels.map(({ tick, text }) => {
        const share = (tick - layout.min) / (layout.max - layout.min)
        return (
          <span
            key={tick}
            aria-hidden="true"
            className={cn(
              "absolute text-xs leading-4 whitespace-nowrap text-muted-foreground tabular-nums",
              vertical ? "right-full pr-2" : "top-full pt-2"
            )}
            style={
              vertical
                ? {
                    top: height - share * height,
                    transform: "translateY(-50%)",
                  }
                : { left: share * width, transform: "translateX(-50%)" }
            }
          >
            {text}
          </span>
        )
      })}
    </>
  )
}

function Tooltip({
  model,
  pointer,
  mode,
  plot,
}: {
  model: BarchartModel
  pointer: Pointer
  mode: Exclude<BarchartTooltip, "hidden">
  plot: Size
}) {
  const row = model.bars[pointer.category]
  const indexes =
    mode === "single"
      ? pointer.bar === null
        ? []
        : [pointer.bar]
      : row.flatMap((bar, index) => (bar.value === null ? [] : [index]))
  if (indexes.length === 0) return null
  const onRight = pointer.x > plot.width / 2
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.max(0, pointer.y - tooltipOffset),
        ...(onRight
          ? { right: plot.width - pointer.x + tooltipOffset }
          : { left: pointer.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 text-muted-foreground">
        {model.categories[pointer.category].title}
      </div>
      {indexes.map((index) => (
        <div
          key={model.series[index].key}
          className="flex items-center gap-1.5"
        >
          <span
            aria-hidden="true"
            className="size-2 shrink-0"
            style={{ backgroundColor: row[index].color }}
          />
          <span className="min-w-0 flex-1 truncate">
            {model.series[index].name}
          </span>
          <span className="pl-3 tabular-nums">{row[index].shown.text}</span>
        </div>
      ))}
    </div>
  )
}

function Plot({
  model,
  display,
  size,
}: {
  model: BarchartModel
  display: Record<string, unknown>
  size: Size
}) {
  const { resolved: theme } = useTheme()
  const [pointer, setPointer] = useState<Pointer | null>(null)
  const vertical = model.orientation === "vertical"
  const count = model.categories.length
  const widest = Math.max(
    0,
    ...model.categories.map((category) => textWidth(category.label))
  )
  const plotHeight = size.height - padTop - axisThickness
  const labelWidth = Math.min(
    Math.min(widest, maxLabelWidth) + axisGap,
    size.width * sidebarShare
  )
  const tickLength = vertical ? plotHeight : size.width - labelWidth - padRight
  const scale = useMemo(
    () =>
      axisTicks(
        model.min,
        model.max,
        tickLength / (vertical ? tickSpacing : horizontalTickSpacing)
      ),
    [model.min, model.max, tickLength, vertical]
  )
  const decimals = axisDecimals(
    scale.ticks,
    model.axis.unit,
    model.axis.decimals
  )
  const labels = useMemo(
    () =>
      scale.ticks.map((tick) => ({
        tick,
        text: formatValue(tick, model.axis.unit, decimals),
      })),
    [scale.ticks, model.axis.unit, decimals]
  )
  const valueWidth =
    Math.max(0, ...labels.map(({ text }) => textWidth(text))) + axisGap
  const left = vertical ? valueWidth : labelWidth
  const plot = {
    width: size.width - left - padRight,
    height: plotHeight,
  }
  const layout: Layout = {
    orientation: model.orientation,
    plot,
    min: scale.min,
    max: scale.max,
    count,
    slotCount: model.slotCount,
    groupWidth: shareOf(display, "groupWidth", 0.7),
    barWidth: shareOf(display, "barWidth", 0.97),
  }
  const tooltip = tooltipOf(display)
  const mode = showValuesOf(display)
  const textColors = useMemo(() => {
    const colors = new Map<string, string | undefined>()
    for (const row of model.bars) {
      for (const { color } of row) {
        if (colors.has(color)) continue
        const rgb = cssColorToRgb(resolveColor(color))
        colors.set(color, rgb ? readableText(rgb) : undefined)
      }
    }
    return colors
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resolveColor reads the live theme tokens, so a theme flip must recompute the text colors
  }, [model.bars, theme])

  if (plot.width <= 0 || plot.height <= 0 || count === 0) return null

  function onMove(event: MouseEvent<SVGSVGElement>) {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left
    const y = event.clientY - box.top
    setPointer({ ...hitTest(layout, model, x, y), x, y })
  }

  const hit = tooltip === "all" || pointer?.bar != null ? pointer : null
  return (
    <div
      className="absolute"
      style={{ left, top: padTop, width: plot.width, height: plot.height }}
    >
      <svg
        role="img"
        aria-label={`Bar chart of ${count} categories and ${model.series.length} series`}
        width={plot.width}
        height={plot.height}
        viewBox={`0 0 ${plot.width} ${plot.height}`}
        className="block overflow-visible"
        onMouseMove={onMove}
        onMouseLeave={() => setPointer(null)}
      >
        <Bars
          model={model}
          layout={layout}
          ticks={scale.ticks}
          hovered={pointer}
        />
      </svg>
      <ValueAxis layout={layout} labels={labels} />
      <CategoryLabels
        model={model}
        layout={layout}
        labelWidth={labelWidth}
        gutter={left}
      />
      <ValueLabels
        model={model}
        layout={layout}
        mode={mode}
        textColors={textColors}
      />
      {tooltip !== "hidden" && hit ? (
        <Tooltip model={model} pointer={hit} mode={tooltip} plot={plot} />
      ) : null}
    </div>
  )
}

export const BarchartPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const model = useMemo(
    () => barchartModel(frames, fields, display, { width, height }),
    [frames, fields, display, width, height]
  )
  const [measured, plotRef] = useMeasuredSize()
  const requested = legendOf(display)
  const legend =
    requested !== "hidden" && model.series.length > 1 ? requested : null
  const estimate = {
    width: width - (legend === "right" ? 160 : 0),
    height: height - (legend === "bottom" ? 28 : 0),
  }
  const size = measured ?? estimate
  return (
    <div
      data-orientation={model.orientation}
      data-categories={model.categories.length}
      data-series={model.series.length}
      data-legend={legend ?? "hidden"}
      className={cn(
        "flex min-h-0 min-w-0",
        legend === "right" ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      {model.reason ? (
        <p className="m-0 p-3 text-xs text-muted-foreground">{model.reason}</p>
      ) : (
        <>
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div ref={plotRef} className="relative min-h-0 min-w-0 flex-1">
              <Plot model={model} display={display} size={size} />
            </div>
            {model.note ? (
              <p className="m-0 shrink-0 px-3 pb-1 text-xs text-muted-foreground">
                {model.note}
              </p>
            ) : null}
          </div>
          {legend ? <Legend series={model.series} placement={legend} /> : null}
        </>
      )}
    </div>
  )
}

export default BarchartPanel
