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
import { useSeriesLegend } from "../legend/use-series-legend"
import type { PanelRenderer } from "../types"
import { logTicks, scalePosition, symlogTicks } from "../value-axis"
import {
  axisTicks,
  barOptions,
  barRect,
  barSource,
  barchartModel,
  categoryBand,
  hitTest,
  labelHeight,
  labelSpot,
  labelStep,
  legendEntries,
  lineHeightFor,
  rotatedExtent,
  roundedBarPath,
  shareOf,
  showValuesOf,
  textWidthFor,
  tooltipIndexes,
  truncateLabel,
  type BarOptions,
  type BarchartModel,
  type Hit,
  type LabelSpot,
  type Layout,
  type Rect,
} from "./barchart-model"

const padTop = 8
const padRight = 12
const padSide = 12
const axisThickness = 24
const characterWidth = 7
const axisGap = 16
const tickSpacing = 32
const horizontalTickSpacing = 72
const maxLabelWidth = 120
const maxRotatedLabelWidth = 160
const strideLabelWidth = 80
const tooltipOffset = 12
const sidebarShare = 0.4
const edgeGap = 4
const axisLabelThickness = 18
const rotatedPad = 8

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

const Bars = memo(function Bars({
  model,
  layout,
  ticks,
  hovered,
  options,
}: {
  model: BarchartModel
  layout: Layout
  ticks: number[]
  hovered: Hit | null
  options: Pick<BarOptions, "barRadius" | "fullHighlight">
}) {
  const vertical = layout.orientation === "vertical"
  const { width, height } = layout.plot
  const { settings } = model.axis
  const position = (value: number) => {
    const share = scalePosition(value, layout.min, layout.max, layout.scale)
    return vertical ? height - share * height : share * width
  }
  const zero = position(0)
  const edge =
    settings.side === "end" ? (vertical ? width : 0) : vertical ? 0 : height
  const borderStroke = model.axis.color ?? "var(--muted-foreground)"
  const band =
    options.fullHighlight && hovered
      ? categoryBand(layout, hovered.category)
      : null
  return (
    <>
      {settings.grid
        ? ticks
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
            ))
        : null}
      {band ? (
        <rect
          data-part="highlight"
          {...band}
          style={{ fill: "var(--foreground)", opacity: 0.06 }}
        />
      ) : null}
      {model.bars.map((row, category) =>
        row.map((bar, index) => {
          if (bar.value === null) return null
          const series = model.series[index]
          const rect = barRect(layout, category, series.slot, bar)
          const outlined =
            !options.fullHighlight &&
            hovered?.category === category &&
            hovered.bar === index
          const path = bar.roundEnd
            ? roundedBarPath(
                rect,
                layout.orientation,
                bar.end < bar.start,
                options.barRadius
              )
            : null
          const style: CSSProperties = {
            fill: bar.color,
            fillOpacity: series.fillOpacity,
            stroke: outlined
              ? "var(--foreground)"
              : series.borderWidth > 0
                ? bar.color
                : "none",
            strokeWidth: outlined ? 1 : series.borderWidth,
          }
          return path ? (
            <path
              key={`${category}:${series.key}`}
              data-series={series.key}
              d={path}
              style={style}
            />
          ) : (
            <rect
              key={`${category}:${series.key}`}
              data-series={series.key}
              {...rect}
              style={style}
            />
          )
        })
      )}
      {layout.scale.kind === "log" ? null : (
        <line
          data-part="baseline"
          x1={vertical ? 0 : zero}
          x2={vertical ? width : zero}
          y1={vertical ? zero : 0}
          y2={vertical ? zero : height}
          style={{ stroke: "var(--border)", strokeWidth: 1 }}
        />
      )}
      {settings.border && settings.side !== "hidden" ? (
        <line
          data-part="axis-border"
          x1={vertical ? edge : 0}
          x2={vertical ? edge : width}
          y1={vertical ? 0 : edge}
          y2={vertical ? height : edge}
          style={{ stroke: borderStroke, strokeWidth: 1 }}
        />
      ) : null}
    </>
  )
})

function valueLabelStyle(
  rect: Rect,
  orientation: Layout["orientation"],
  plot: Size,
  spot: Exclude<LabelSpot, null>,
  negative: boolean,
  lineHeight: number
): CSSProperties {
  const inside = spot === "inside"
  if (orientation === "vertical") {
    const top = inside
      ? negative
        ? rect.y + rect.height - lineHeight - 2
        : rect.y + 2
      : negative
        ? rect.y + rect.height
        : rect.y - lineHeight
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
  fontSize,
}: {
  model: BarchartModel
  layout: Layout
  mode: ReturnType<typeof showValuesOf>
  textColors: Map<string, string | undefined>
  fontSize: number
}) {
  if (mode === "never") return null
  const lineHeight = lineHeightFor(fontSize)
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
            textWidthFor(bar.shown.text, fontSize),
            { stacked: series.stack !== "none", negative, lineHeight }
          )
          if (spot === null) return null
          return (
            <span
              key={`${category}:${series.key}`}
              aria-hidden="true"
              className="pointer-events-none absolute whitespace-nowrap tabular-nums"
              style={{
                fontSize,
                lineHeight: `${lineHeight}px`,
                ...valueLabelStyle(
                  rect,
                  layout.orientation,
                  layout.plot,
                  spot,
                  negative,
                  lineHeight
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
  options,
}: {
  model: BarchartModel
  layout: Layout
  labelWidth: number
  gutter: number
  options: Pick<BarOptions, "xTickRotation" | "xTickSpacing" | "xTickMaxLength">
}) {
  const vertical = layout.orientation === "vertical"
  const across = vertical ? layout.plot.width : layout.plot.height
  const slot = across / layout.count
  const rotation = vertical ? options.xTickRotation : 0
  const labels = model.categories.map((category) =>
    truncateLabel(category.label, options.xTickMaxLength)
  )
  const widest = Math.max(0, ...labels.map(textWidth))
  const step = labelStep(
    layout.count,
    across,
    vertical ? Math.min(widest, strideLabelWidth) + 8 : labelHeight,
    options.xTickSpacing
  )
  return (
    <>
      {model.categories.map((category, index) => {
        if (index % step !== 0) return null
        const center = index * slot + slot / 2
        const text = labels[index]
        if (rotation !== 0) {
          const toRight = rotation > 0
          return (
            <span
              key={category.key}
              title={category.title}
              aria-hidden="true"
              className="absolute text-xs leading-4 whitespace-nowrap text-muted-foreground"
              style={{
                left: center,
                top: layout.plot.height + rotatedPad,
                transformOrigin: toRight ? "right top" : "left top",
                transform: `translateX(${toRight ? "-100%" : "0"}) rotate(${-rotation}deg)`,
              }}
            >
              {text}
            </span>
          )
        }
        const half = Math.min(
          (slot * step) / 2,
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
            {text}
          </span>
        )
      })}
    </>
  )
}

function ValueAxis({
  model,
  layout,
  labels,
  extent,
}: {
  model: BarchartModel
  layout: Layout
  labels: { tick: number; text: string }[]
  extent: number
}) {
  const { settings, color } = model.axis
  if (settings.side === "hidden") return null
  const vertical = layout.orientation === "vertical"
  const atEnd = settings.side === "end"
  const { width, height } = layout.plot
  const tone = color ? { color } : undefined
  return (
    <>
      {labels.map(({ tick, text }) => {
        const share = scalePosition(tick, layout.min, layout.max, layout.scale)
        return (
          <span
            key={tick}
            aria-hidden="true"
            className={cn(
              "absolute text-xs leading-4 whitespace-nowrap tabular-nums",
              color ? undefined : "text-muted-foreground",
              vertical
                ? atEnd
                  ? "left-full pl-2"
                  : "right-full pr-2"
                : atEnd
                  ? "bottom-full pb-2"
                  : "top-full pt-2"
            )}
            style={{
              ...tone,
              ...(vertical
                ? {
                    top: height - share * height,
                    transform: "translateY(-50%)",
                  }
                : { left: share * width, transform: "translateX(-50%)" }),
            }}
          >
            {text}
          </span>
        )
      })}
      {settings.label ? (
        <span
          data-part="axis-label"
          aria-hidden="true"
          className="absolute text-center text-xs leading-[18px] whitespace-nowrap text-muted-foreground"
          style={
            vertical
              ? {
                  writingMode: "vertical-rl",
                  transform: atEnd ? undefined : "rotate(180deg)",
                  top: 0,
                  height,
                  ...(atEnd
                    ? { left: width + extent - axisLabelThickness }
                    : { left: -extent }),
                }
              : {
                  left: 0,
                  width,
                  ...(atEnd
                    ? { top: -extent }
                    : { top: height + extent - axisLabelThickness }),
                }
          }
        >
          {settings.label}
        </span>
      ) : null}
    </>
  )
}

function Tooltip({
  model,
  pointer,
  indexes,
  plot,
  options,
}: {
  model: BarchartModel
  pointer: Pointer
  indexes: number[]
  plot: Size
  options: Pick<BarOptions, "tooltipMaxWidth" | "tooltipMaxHeight">
}) {
  const row = model.bars[pointer.category]
  const onRight = pointer.x > plot.width / 2
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 min-w-36 overflow-hidden rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.max(0, pointer.y - tooltipOffset),
        maxWidth: options.tooltipMaxWidth,
        maxHeight: options.tooltipMaxHeight,
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
  options,
  size,
}: {
  model: BarchartModel
  display: Record<string, unknown>
  options: BarOptions
  size: Size
}) {
  const { resolved: theme } = useTheme()
  const [pointer, setPointer] = useState<Pointer | null>(null)
  const vertical = model.orientation === "vertical"
  const { settings } = model.axis
  const count = model.categories.length
  const labels = useMemo(
    () =>
      model.categories.map((category) =>
        truncateLabel(category.label, options.xTickMaxLength)
      ),
    [model.categories, options.xTickMaxLength]
  )
  const widest = Math.max(0, ...labels.map(textWidth))
  const rotation = vertical ? options.xTickRotation : 0
  const categoryExtent =
    rotation === 0
      ? axisThickness
      : rotatedExtent(
          Math.min(widest, maxRotatedLabelWidth),
          rotation,
          labelHeight
        ) + rotatedPad
  const labelWidth = Math.min(
    Math.min(widest, maxLabelWidth) + axisGap,
    size.width * sidebarShare
  )
  const hiddenAxis = settings.side === "hidden"
  const atEnd = settings.side === "end"
  const axisLabel = settings.label && !hiddenAxis ? axisLabelThickness : 0
  const measureLength = vertical
    ? size.height - padTop - categoryExtent
    : size.width - labelWidth - padRight
  const scale = useMemo(() => {
    const maxTicks =
      measureLength / (vertical ? tickSpacing : horizontalTickSpacing)
    const { kind, logBase, linearThreshold } = model.scale
    if (kind === "log") {
      return {
        min: model.min,
        max: model.max,
        ticks: logTicks(model.min, model.max, logBase, maxTicks),
      }
    }
    if (kind === "symlog") {
      return {
        min: model.min,
        max: model.max,
        ticks: symlogTicks(model.min, model.max, linearThreshold, maxTicks),
      }
    }
    return axisTicks(model.min, model.max, maxTicks)
  }, [model.min, model.max, model.scale, measureLength, vertical])
  const decimals = axisDecimals(
    scale.ticks,
    model.axis.unit,
    model.axis.decimals
  )
  const labelTexts = useMemo(
    () =>
      scale.ticks.map((tick) => ({
        tick,
        text: formatValue(tick, model.axis.unit, decimals),
      })),
    [scale.ticks, model.axis.unit, decimals]
  )
  const valueTextWidth =
    Math.max(0, ...labelTexts.map(({ text }) => textWidth(text))) + axisGap
  const naturalExtent = vertical ? valueTextWidth : axisThickness
  const valueExtent = hiddenAxis
    ? 0
    : (settings.width ?? naturalExtent) + axisLabel
  const left = vertical
    ? atEnd || hiddenAxis
      ? padSide
      : valueExtent
    : labelWidth
  const right = vertical ? (atEnd ? valueExtent : padRight) : padRight
  const top = vertical ? padTop : atEnd ? valueExtent : padTop
  const bottom = vertical
    ? categoryExtent
    : hiddenAxis || atEnd
      ? padTop
      : valueExtent
  const plot = {
    width: size.width - left - right,
    height: size.height - top - bottom,
  }
  const layout: Layout = {
    orientation: model.orientation,
    plot,
    min: scale.min,
    max: scale.max,
    scale: model.scale,
    count,
    slotCount: model.slotCount,
    groupWidth: shareOf(display, "groupWidth", 0.7),
    barWidth: shareOf(display, "barWidth", 0.97),
  }
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

  const hit = options.tooltip === "all" || pointer?.bar != null ? pointer : null
  const indexes =
    options.tooltip !== "hidden" && hit
      ? tooltipIndexes(model, hit, options)
      : []
  return (
    <div
      className="absolute"
      style={{ left, top, width: plot.width, height: plot.height }}
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
          options={options}
        />
      </svg>
      <ValueAxis
        model={model}
        layout={layout}
        labels={labelTexts}
        extent={valueExtent}
      />
      <CategoryLabels
        model={model}
        layout={layout}
        labelWidth={labelWidth}
        gutter={left}
        options={options}
      />
      <ValueLabels
        model={model}
        layout={layout}
        mode={mode}
        textColors={textColors}
        fontSize={options.valueSize}
      />
      {hit && indexes.length > 0 ? (
        <Tooltip
          model={model}
          pointer={hit}
          indexes={indexes}
          plot={plot}
          options={options}
        />
      ) : null}
    </div>
  )
}

const BarchartPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
  stateScope,
}) => {
  const options = useMemo(() => barOptions(display), [display])
  const source = useMemo(
    () => barSource(frames, fields, display),
    [frames, fields, display]
  )
  const entries = useMemo(() => legendEntries(source), [source])
  const legend = useSeriesLegend({
    stateKey: `${stateScope}:${panel.id}`,
    entries,
    options,
    width,
    height,
  })
  const model = useMemo(
    () => barchartModel(source, display, { width, height }, legend.hidden),
    [source, display, width, height, legend.hidden]
  )
  const [measured, plotRef] = useMeasuredSize()
  const estimate = {
    width: width - legend.width,
    height: height - (legend.placement === "bottom" ? 28 : 0),
  }
  const size = measured ?? estimate
  return (
    <div
      data-orientation={model.orientation}
      data-categories={model.categories.length}
      data-series={model.series.length}
      data-legend={legend.placement ?? "hidden"}
      className={cn(
        "flex min-h-0 min-w-0",
        legend.placement === "right" ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      {model.reason ? (
        <p className="m-0 p-3 text-xs text-muted-foreground">{model.reason}</p>
      ) : (
        <>
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div ref={plotRef} className="relative min-h-0 min-w-0 flex-1">
              <Plot
                model={model}
                display={display}
                options={options}
                size={size}
              />
            </div>
            {model.note ? (
              <p className="m-0 shrink-0 px-3 pb-1 text-xs text-muted-foreground">
                {model.note}
              </p>
            ) : null}
          </div>
          {legend.node}
        </>
      )}
    </div>
  )
}

export default BarchartPanel
