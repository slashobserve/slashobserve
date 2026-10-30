import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react"
import uPlot from "uplot"
import "uplot/dist/uPlot.min.css"

import type { PanelDataMeta, WireFrame } from "@/dashboards/queries"
import { stableJson } from "@/lib/json"
import { formatDateTime } from "@/lib/time"
import { timeLabel } from "@/lib/time-ticks"
import { useTheme } from "@/lib/theme"
import { axisDecimals, formatValue } from "@/lib/units"
import { cn } from "@/lib/utils"
import type { DataPanel } from "@/spec"

import { readChartColors, resolveColor } from "../colors"
import type { FieldConfig } from "../field-config"
import { dataRenderers } from "../index"
import { reducerLabels } from "../reducers"
import { thresholdColor } from "../thresholds"
import type { CursorMember } from "../shared-cursor"
import type { PanelRenderer, PanelRendererProps } from "../types"
import { Unsupported } from "../unsupported"
import { alignFrames, type AlignedFrames } from "./align"
import { Legend, rootFontSize, type LegendItem } from "../legend/legend"
import { effectiveLegendMode, legendOrder } from "../legend/legend-model"
import { LegendTable, useLegendTextMeasure } from "../legend/legend-table"
import { legendTableWidth, rightLegendWidth } from "../legend/legend-width"
import {
  chartSeries,
  configuredKeys,
  ghostsDrawn,
  legendCells,
  peerCursorAt,
  plotStateFor,
  thresholdsFor,
  timeAxis,
  type ChartSeries,
  type PlotState,
  type ThresholdLines,
  type XAxis,
} from "./plot-model"
import { exemplarPoints } from "./exemplars"
import {
  cursorPlugin,
  exemplarsPlugin,
  thresholdsPlugin,
  tooltipPlugin,
  type ExemplarLayer,
  type TooltipSeries,
  type TooltipView,
} from "./plugins"
import { scaleDistribution } from "./axis-scale"
import { isolatedPoints } from "./points"
import {
  axesInUse,
  axisRange,
  fillPercent,
  fillStyle,
  firstStackMode,
  scaleOf,
  settingsOf,
  timeseriesDisplay,
  type AxesInUse,
  type AxisKey,
  type AxisSettings,
  type Interpolation,
  type TimeseriesDisplay,
} from "./series-config"
import { emptySeriesState, useSeriesStore, visibleSeries } from "./series-state"
import "./timeseries.css"

const legendShare = 0.4
const minSelectWidth = 5
const axisFontSize = 12
const axisLabelSize = 18
const noSelect = { left: 0, top: 0, width: 0, height: 0 }
const barSize: [number, number] = [0.6, 64]
const ghostDash = [2, 3]
const noteHeight = 16
const noExemplars: ExemplarLayer = { points: [], seriesKeys: [] }

const pathBuilders: Record<Interpolation, uPlot.Series.PathBuilder> = {
  linear: uPlot.paths.linear!(),
  smooth: uPlot.paths.spline!(),
  stepBefore: uPlot.paths.stepped!({ align: -1 }),
  stepAfter: uPlot.paths.stepped!({ align: 1 }),
}

type Latest = {
  model: Model
  state: PlotState
  entries: TooltipSeries[]
  exemplars: ExemplarLayer
  meta: PanelDataMeta
  width: number
  chartHeight: number
  onTimeRangeSelect: PanelRendererProps["onTimeRangeSelect"]
}

type Model = {
  series: ChartSeries[]
  axes: AxesInUse
  panelAxis: AxisSettings
  ghostCount: number
  thresholds: ThresholdLines | null
  tooltip: "single" | "all" | "hidden"
  tooltipView: TooltipView
  xAxisShown: boolean
}

type PlotConfig = {
  member: { current: CursorMember | null }
  peerTooltip: { current: boolean }
  host: HTMLElement
  latest: { current: Latest }
  xAxis: XAxis
  draw?: uPlot.Plugin
}

const tickEdgeGap = 24

function timeLabels(splits: (number | null)[], spanSeconds: number): string[] {
  const spanMs = spanSeconds * 1000
  return splits.map((seconds) =>
    seconds === null ? "" : timeLabel(seconds * 1000, spanMs)
  )
}

const dropEdgeTicks: uPlot.Axis.Filter = (u, splits) =>
  splits.map((seconds) => {
    const x = u.valToPos(seconds, "x")
    return x < tickEdgeGap || x > u.bbox.width / uPlot.pxRatio - tickEdgeGap
      ? null
      : seconds
  })

function linePoints(entry: ChartSeries, stroke: string): uPlot.Series.Points {
  if (entry.points === "never") return { show: false }
  if (entry.points === "always") {
    return {
      show: true,
      size: entry.pointSize,
      width: 0,
      fill: stroke,
      stroke,
    }
  }
  return {
    show: false,
    filter: isolatedPoints,
    size: 8,
    fill: stroke,
    stroke,
  }
}

function seriesOptions(
  entry: ChartSeries,
  hidden: ReadonlySet<string>,
  customPainted: boolean
): uPlot.Series {
  const stroke = resolveColor(entry.color)
  const fill = fillPercent(entry.style, entry.fill)
  const base = {
    label: entry.label,
    scale: scaleOf[entry.axis],
    show: !hidden.has(entry.key),
    stroke,
    dash: entry.dash,
    spanGaps: entry.spanNulls === true,
  }
  if (customPainted) {
    return { ...base, width: 0, paths: () => null, points: { show: false } }
  }
  if (entry.style === "bars") {
    return {
      ...base,
      width: 1,
      fill: fillStyle(stroke, fill),
      paths: uPlot.paths.bars!({ size: barSize }),
      points: { show: false },
    }
  }
  if (entry.style === "points") {
    return {
      ...base,
      width: 2,
      paths: () => null,
      points: {
        show: true,
        size: entry.pointSize,
        width: 0,
        fill: stroke,
        stroke,
      },
    }
  }
  return {
    ...base,
    width: entry.lineWidth,
    paths: pathBuilders[entry.interpolation],
    ...(fill > 0 ? { fill: fillStyle(stroke, fill) } : {}),
    points: linePoints(entry, stroke),
  }
}

function createPlot(container: HTMLElement, config: PlotConfig): uPlot {
  const { member, peerTooltip, host, latest, xAxis, draw } = config
  const timed = xAxis.kind === "time"
  const customPainted = draw !== undefined
  const { model } = latest.current
  const { series, axes, panelAxis, ghostCount } = model
  const { state, width, chartHeight } = latest.current
  const first = 1 + ghostCount
  const colors = readChartColors()
  const font = `${axisFontSize}px ${colors.font}`
  const ghostColor = resolveColor("var(--ghost)")
  const borderColor = resolveColor("var(--chart-axis)")
  const leftSettings = axes.left ?? panelAxis

  const plugins: uPlot.Plugin[] = []
  if (model.tooltip !== "hidden") {
    plugins.push(
      tooltipPlugin(
        host,
        model.tooltip,
        model.tooltipView,
        first,
        () => latest.current.entries,
        (x) =>
          xAxis.kind === "time" ? formatDateTime(x * 1000) : xAxis.format(x),
        () => peerTooltip.current,
        () => latest.current.exemplars
      )
    )
  }
  if (model.thresholds) {
    plugins.push(
      thresholdsPlugin(model.thresholds.list, model.thresholds.mode, (color) =>
        resolveColor(thresholdColor(color))
      )
    )
  }
  if (timed && !customPainted) {
    plugins.push(
      exemplarsPlugin(
        first,
        () => latest.current.exemplars,
        () => latest.current.entries
      )
    )
  }
  if (draw) plugins.push(draw)
  if (timed) plugins.push(cursorPlugin(member))

  const valueAxis = (
    key: AxisKey,
    side: 1 | 3,
    settings: AxisSettings,
    defaultGrid: boolean,
    show: boolean
  ): uPlot.Axis => {
    const onAxis = series.find((entry) => entry.axis === key)
    const stroke =
      settings.color === "series" && onAxis
        ? resolveColor(onAxis.color)
        : colors.muted
    return {
      scale: scaleOf[key],
      side,
      show,
      stroke,
      font,
      ...(settings.label
        ? {
            label: settings.label,
            labelSize: axisLabelSize,
            labelFont: font,
          }
        : {}),
      grid: {
        show: settings.grid ?? defaultGrid,
        stroke: colors.grid,
        width: 1,
      },
      ...(settings.border
        ? { border: { show: true, stroke: borderColor, width: 1 } }
        : {}),
      ticks: { show: false },
      values: (_u, splits) => {
        const decimals = axisDecimals(splits, settings.unit, settings.decimals)
        return splits.map((value) =>
          formatValue(value, settings.unit, decimals)
        )
      },
      size:
        settings.width ??
        ((u, values) => {
          if (!values) return 48
          u.ctx.font = `${axisFontSize * uPlot.pxRatio}px ${colors.font}`
          const widest = Math.max(
            ...values.map((text) => u.ctx.measureText(text).width)
          )
          return Math.ceil(widest / uPlot.pxRatio) + 14
        }),
    }
  }

  const valueScale = (settings: AxisSettings): uPlot.Scale => ({
    ...(settings.percent
      ? {}
      : scaleDistribution(
          settings.scale,
          settings.logBase,
          settings.linearThreshold
        )),
    range: (_u, min, max) => axisRange(settings, min, max),
  })

  const leftShown = axes.left !== undefined || axes.right === undefined
  const options: uPlot.Options = {
    width,
    height: chartHeight,
    padding: [8, 12, 0, 0],
    legend: { show: false },
    scales: {
      x: {
        time: timed,
        range: (_u, dataMin, dataMax) => {
          const { meta } = latest.current
          const from = Date.parse(meta.from) / 1000
          const to = Date.parse(meta.to) / 1000
          if (
            timed &&
            Number.isFinite(from) &&
            Number.isFinite(to) &&
            to > from
          ) {
            return [from, to]
          }
          return dataMin === dataMax
            ? [dataMin - 1, dataMax + 1]
            : [dataMin, dataMax]
        },
      },
      y: valueScale(leftSettings),
      ...(axes.right ? { y2: valueScale(axes.right) } : {}),
      ...(axes.hidden ? { y3: valueScale(axes.hidden) } : {}),
    },
    axes: [
      {
        show: model.xAxisShown,
        stroke: colors.muted,
        font,
        size: model.xAxisShown ? 28 : 0,
        grid: { stroke: colors.grid, width: 1, filter: dropEdgeTicks },
        ticks: { show: false },
        filter: dropEdgeTicks,
        values: (u, splits) =>
          xAxis.kind === "time"
            ? timeLabels(splits, (u.scales.x.max ?? 0) - (u.scales.x.min ?? 0))
            : splits.map((value) =>
                value === null ? "" : xAxis.format(value)
              ),
      },
      valueAxis("left", 3, leftSettings, leftShown, leftShown),
      ...(axes.right
        ? [valueAxis("right", 1, axes.right, !leftShown, true)]
        : []),
    ],
    series: [
      {},
      ...Array.from({ length: ghostCount }, () => ({
        label: "Saved",
        scale: "y",
        stroke: ghostColor,
        dash: ghostDash,
        width: 2,
        spanGaps: false,
        points: { show: false },
      })),
      ...series.map((entry) =>
        seriesOptions(entry, state.hidden, customPainted)
      ),
    ],
    bands: state.bands.map((band) => ({ ...band })),
    cursor: {
      drag: { x: timed, y: false, setScale: false },
      points: {
        size: (_u, index) => (index < first || customPainted ? 0 : 6),
      },
    },
    hooks: timed
      ? {
          setSelect: [
            (u) => {
              const { left, width: selected } = u.select
              if (selected >= minSelectWidth) {
                const from = u.posToVal(left, "x") * 1000
                const to = u.posToVal(left + selected, "x") * 1000
                latest.current.onTimeRangeSelect(
                  Math.round(from),
                  Math.round(to)
                )
              }
              u.setSelect(noSelect, false)
            },
          ],
        }
      : {},
    plugins,
  }

  return new uPlot(options, state.data, container)
}

function modelFor(
  panel: DataPanel,
  series: ChartSeries[],
  ghostCount: number,
  options: Pick<
    TimeseriesDisplay,
    | "tooltip"
    | "tooltipSort"
    | "tooltipHideZeros"
    | "tooltipCumulative"
    | "tooltipMaxWidth"
    | "tooltipMaxHeight"
    | "xAxis"
  >,
  tightRange: boolean
): Model {
  const axes = axesInUse(
    series,
    series.map(({ config }) => config),
    panel,
    ghostCount
  )
  if (tightRange && axes.left) axes.left = { ...axes.left, tight: true }
  return {
    series,
    axes,
    panelAxis: settingsOf(panel),
    ghostCount,
    thresholds: thresholdsFor(series, axes.left, panel),
    tooltip: options.tooltip,
    tooltipView: {
      sort: options.tooltipSort,
      hideZeros: options.tooltipHideZeros,
      cumulative: options.tooltipCumulative,
      maxWidth: options.tooltipMaxWidth,
      maxHeight: options.tooltipMaxHeight,
    },
    xAxisShown: options.xAxis !== "hidden",
  }
}

function exemplarRange(
  meta: PanelDataMeta,
  xs: readonly number[]
): [number, number] {
  const from = Date.parse(meta.from) / 1000
  const to = Date.parse(meta.to) / 1000
  if (Number.isFinite(from) && Number.isFinite(to) && to > from) {
    return [from, to]
  }
  return [xs[0] ?? 0, xs[xs.length - 1] ?? 0]
}

function exemplarLayerFor(
  frames: readonly WireFrame[],
  aligned: AlignedFrames,
  series: ChartSeries[],
  meta: PanelDataMeta
): ExemplarLayer {
  if (frames.length === 0) return noExemplars
  const alignedByKey = new Map(
    aligned.series.map((entry) => [entry.key, entry])
  )
  const matchable = series.flatMap((entry) => {
    const source = alignedByKey.get(entry.key)
    return source
      ? [
          {
            key: entry.key,
            ref: source.frame.ref,
            labels: source.frame.labels,
            scale: scaleOf[entry.axis],
          },
        ]
      : []
  })
  return {
    points: exemplarPoints(frames, matchable, exemplarRange(meta, aligned.xs)),
    seriesKeys: series.map((entry) => entry.key),
  }
}

export type ChartProps = PanelRendererProps & {
  aligned: AlignedFrames
  configs: FieldConfig[]
  xAxis?: XAxis
  draw?: uPlot.Plugin
  rowColor?: (row: number) => string
}

export function TimeseriesChart({
  panel,
  display,
  meta,
  width,
  height,
  stateScope,
  cursor,
  onTimeRangeSelect,
  aligned,
  configs,
  xAxis = timeAxis,
  draw,
  rowColor,
  exemplars,
}: ChartProps) {
  const { resolved: theme } = useTheme()
  const hostRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const legendRef = useRef<HTMLDivElement>(null)
  const plotRef = useRef<uPlot | null>(null)
  const appliedRef = useRef<PlotState | null>(null)
  const [legendHeight, setLegendHeight] = useState(0)

  const options = useMemo(() => timeseriesDisplay(display), [display])
  const reducers = options.legendValues
  const displayKey = stableJson(display)
  const configsKey = stableJson(configs)
  const thresholdsKey = stableJson(panel.thresholds ?? null)
  const panelAxisKey = stableJson(settingsOf(panel))

  const stateKey = `${stateScope}:${panel.id}`
  const stored = useSeriesStore((store) => store.panels[stateKey])
  const commitKeys = useSeriesStore((store) => store.commit)
  const toggleKey = useSeriesStore((store) => store.toggle)
  const { slots, hidden: storedHidden } = stored ?? emptySeriesState

  const keys = useMemo(
    () => configuredKeys(aligned.series, configs),
    [aligned.series, configs]
  )
  const series = useMemo(
    () => chartSeries(aligned.series, configs, slots),
    [aligned.series, configs, slots]
  )

  const legendMode = effectiveLegendMode(options.legendMode, reducers)
  const order = useMemo(
    () =>
      legendOrder(
        series.map(({ key, label, fieldValues }) => ({
          key,
          label,
          values: fieldValues,
        })),
        {
          mode: legendMode,
          values: reducers,
          sortBy: options.legendSortBy,
          sortDesc: options.legendSortDesc,
          hideZero: options.legendHideZero,
          hideEmpty: options.legendHideEmpty,
        }
      ),
    [series, legendMode, reducers, options]
  )
  const listed = useMemo(
    () => order.map((index) => series[index]),
    [order, series]
  )
  const legendShown =
    series.length > 1 && options.legend !== "hidden" && order.length > 0
  const placement = options.legend === "right" ? "right" : "bottom"
  const hidden = useMemo(
    () => visibleSeries(storedHidden, keys, legendShown),
    [storedHidden, keys, legendShown]
  )
  const cells = useMemo(() => legendCells(listed, reducers), [listed, reducers])
  const rightLegend = legendShown && placement === "right"
  const textMeasure = useLegendTextMeasure()
  const tableWidth = useMemo(() => {
    if (!rightLegend || reducers.length === 0) return null
    const rem = rootFontSize()
    return legendTableWidth(
      listed.map(({ label }) => label),
      reducers.map((reducer) => reducerLabels[reducer]),
      cells,
      textMeasure(rem),
      rem
    )
  }, [rightLegend, listed, reducers, cells, textMeasure])
  const legendWidth = rightLegend
    ? rightLegendWidth(tableWidth, width, options.legendWidth)
    : 0
  const chartWidth = width - legendWidth
  const bottomLegend = legendShown && placement === "bottom"
  const exemplarsDrawn = xAxis.kind === "time" && draw === undefined
  const exemplarsTruncated =
    exemplarsDrawn && meta.queries.some((q) => q.exemplars === "truncated")
  const chartHeight = Math.max(
    0,
    height -
      (bottomLegend ? legendHeight : 0) -
      (exemplarsTruncated ? noteHeight : 0)
  )
  const ready = chartWidth > 0 && chartHeight > 0
  const signature = series
    .map((entry) => `${entry.key}\u0001${entry.label}`)
    .join("\u0000")

  const ghosts = useMemo(
    () => ghostsDrawn(series, aligned.ghosts),
    [series, aligned.ghosts]
  )

  const { state, entries } = useMemo(() => {
    const { state, below, stacked } = plotStateFor(
      aligned.xs,
      ghosts,
      series,
      hidden
    )
    return {
      state,
      entries: series.map((entry, index): TooltipSeries => ({
        label: entry.label,
        color: entry.color,
        line: entry.line,
        style: entry.style,
        scale: scaleOf[entry.axis],
        config: entry.config,
        values: entry.values,
        stacked: stacked[index],
        below: below[index],
        rowColor,
      })),
    }
  }, [aligned.xs, ghosts, series, hidden, rowColor])

  const exemplarLayer = useMemo(
    () =>
      exemplarsDrawn
        ? exemplarLayerFor(exemplars ?? [], aligned, series, meta)
        : noExemplars,
    [exemplarsDrawn, exemplars, aligned, series, meta]
  )

  const model = modelFor(
    panel,
    series,
    ghosts.length,
    options,
    draw !== undefined
  )

  const latest = useRef<Latest>({
    model,
    state,
    entries,
    exemplars: exemplarLayer,
    meta,
    width: chartWidth,
    chartHeight,
    onTimeRangeSelect,
  })
  useLayoutEffect(() => {
    latest.current = {
      model,
      state,
      entries,
      exemplars: exemplarLayer,
      meta,
      width: chartWidth,
      chartHeight,
      onTimeRangeSelect,
    }
  })

  useLayoutEffect(() => {
    commitKeys(stateKey, keys)
  }, [commitKeys, stateKey, keys])

  useLayoutEffect(() => {
    const element = legendRef.current
    if (!element || !bottomLegend) return
    const observer = new ResizeObserver(() =>
      setLegendHeight(Math.ceil(element.getBoundingClientRect().height))
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [bottomLegend, legendMode, reducers.length])

  useLayoutEffect(() => {
    const container = containerRef.current
    const host = hostRef.current
    if (!container || !host || !ready) return
    const member = { current: null as CursorMember | null }
    const peerTooltip = { current: false }
    const plot = createPlot(container, {
      member,
      peerTooltip,
      host,
      latest,
      xAxis,
      draw,
    })
    if (xAxis.kind === "time") {
      member.current = cursor.join((peer) => {
        const width = plot.bbox.width / uPlot.pxRatio
        const height = plot.bbox.height / uPlot.pxRatio
        const position = peerCursorAt(
          peer ? plot.valToPos(peer.x / 1000, "x") : null,
          width,
          height
        )
        peerTooltip.current =
          peer !== null && peer.tooltip && position.left >= 0
        plot.setCursor(position)
      })
    }
    plotRef.current = plot
    appliedRef.current = latest.current.state
    return () => {
      member.current?.leave()
      plot.destroy()
      plotRef.current = null
      appliedRef.current = null
    }
  }, [
    ready,
    signature,
    theme,
    cursor,
    xAxis,
    draw,
    configsKey,
    displayKey,
    ghosts.length,
    panelAxisKey,
    panel.thresholdMode,
    thresholdsKey,
  ])

  useLayoutEffect(() => {
    const plot = plotRef.current
    if (!plot || appliedRef.current === state) return
    const first = 1 + ghosts.length
    plot.batch(() => {
      series.forEach((entry, index) => {
        const show = !state.hidden.has(entry.key)
        if (plot.series[first + index]?.show !== show) {
          plot.setSeries(first + index, { show })
        }
      })
      plot.delBand(null)
      state.bands.forEach((band) => plot.addBand({ ...band }))
      plot.setData(state.data)
    })
    appliedRef.current = state
  }, [state, series, ghosts.length])

  useLayoutEffect(() => {
    plotRef.current?.redraw(false)
  }, [exemplarLayer])

  useLayoutEffect(() => {
    const plot = plotRef.current
    if (!plot || !ready) return
    if (plot.width !== chartWidth || plot.height !== chartHeight) {
      plot.setSize({ width: chartWidth, height: chartHeight })
    }
  }, [ready, chartWidth, chartHeight])

  const toggle = useCallback(
    (key: string) => toggleKey(stateKey, key),
    [toggleKey, stateKey]
  )

  const items: LegendItem[] = listed.map((entry) => ({
    key: entry.key,
    label: entry.label,
    color: entry.color,
    line: entry.line,
    style: entry.style,
  }))
  const legendProps = {
    ref: legendRef,
    items,
    hidden,
    placement,
    available: height * legendShare,
    width: legendWidth,
    onToggle: toggle,
  } as const
  const axisNames = (["left", "right", "hidden"] as const).filter(
    (name) => model.axes[name]
  )
  const axisLabels = axisNames.flatMap((name) => model.axes[name]?.label ?? [])
  const exemplarCount = exemplarLayer.points.length
  const exemplarSuffix =
    exemplarCount > 0
      ? `, ${exemplarCount} ${exemplarCount === 1 ? "exemplar" : "exemplars"}`
      : ""
  const plotName =
    xAxis.kind === "time"
      ? `${panel.title}: ${series.length} series${exemplarSuffix}, ${formatDateTime(meta.from)} to ${formatDateTime(meta.to)}`
      : `${panel.title}: ${series.length} series`

  return (
    <div
      ref={hostRef}
      className={cn(
        "relative flex",
        placement === "right" && legendShown ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      <div className="flex shrink-0 flex-col" style={{ width: chartWidth }}>
        <div
          ref={containerRef}
          role="img"
          aria-label={
            axisLabels.length > 0
              ? `${plotName}, axes: ${axisLabels.join(", ")}`
              : plotName
          }
          data-axes={axisNames.join(" ")}
          data-series={series.length}
          data-ghosts={ghosts.length}
          data-stack={firstStackMode(series)}
          className="ts-chart shrink-0 overflow-hidden"
          style={{ width: chartWidth, height: chartHeight }}
        />
        {exemplarsTruncated ? (
          <p
            className="truncate text-xs leading-4 text-muted-foreground"
            style={{ height: noteHeight }}
          >
            Showing the newest 2,000 exemplars.
          </p>
        ) : null}
      </div>
      {legendShown && legendMode === "table" ? (
        <LegendTable
          {...legendProps}
          reducers={reducers}
          cells={cells}
          sortBy={options.legendSortBy}
          sortDesc={options.legendSortDesc}
        />
      ) : null}
      {legendShown && legendMode === "list" ? (
        <Legend {...legendProps} reducers={reducers} cells={cells} />
      ) : null}
    </div>
  )
}

const TimeseriesPanel: PanelRenderer = (props) => {
  const { frames, ghostFrames, fields } = props
  const aligned = useMemo(
    () => alignFrames(frames, fields.displayName, ghostFrames),
    [frames, fields, ghostFrames]
  )
  const configs = useMemo(
    () =>
      aligned.series.map(({ frame, field }) => fields.configFor(frame, field)),
    [aligned.series, fields]
  )
  if (aligned.series.length === 0 || aligned.xs.length === 0) {
    const Table = dataRenderers.table
    return Table ? <Table {...props} /> : <Unsupported />
  }
  return <TimeseriesChart {...props} aligned={aligned} configs={configs} />
}

export default TimeseriesPanel
