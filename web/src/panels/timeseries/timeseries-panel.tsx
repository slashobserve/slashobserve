import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react"
import uPlot from "uplot"
import "uplot/dist/uPlot.min.css"

import type { PanelDataMeta } from "@/dashboards/queries"
import { stableJson } from "@/lib/json"
import { formatDateTime } from "@/lib/time"
import { timeLabel } from "@/lib/time-ticks"
import { useTheme } from "@/lib/theme"
import { axisDecimals, formatAxisValue } from "@/lib/units"
import { cn } from "@/lib/utils"
import type { DataPanel } from "@/spec"

import { readChartColors, resolveColor } from "../colors"
import type { FieldConfig } from "../field-config"
import { dataRenderers } from "../index"
import { reducerLabels } from "../reducers"
import { thresholdColor } from "../thresholds"
import type { PanelRenderer, PanelRendererProps } from "../types"
import { Unsupported } from "../unsupported"
import { alignFrames, type AlignedFrames } from "./align"
import { Legend, rootFontSize, type LegendItem } from "./legend"
import { LegendTable, useLegendTextMeasure } from "./legend-table"
import { legendTableWidth, rightLegendWidth } from "./legend-width"
import {
  chartSeries,
  configuredKeys,
  ghostsDrawn,
  legendCells,
  plotStateFor,
  thresholdsFor,
  timeAxis,
  type ChartSeries,
  type PlotState,
  type ThresholdLines,
  type XAxis,
} from "./plot-model"
import { thresholdsPlugin, tooltipPlugin, type TooltipSeries } from "./plugins"
import { isolatedPoints } from "./points"
import {
  axesInUse,
  axisRange,
  fillPercent,
  fillStyle,
  firstStackMode,
  scaleOf,
  timeseriesDisplay,
  type AxesInUse,
  type AxisKey,
  type AxisSettings,
} from "./series-config"
import { emptySeriesState, useSeriesStore, visibleSeries } from "./series-state"
import "./timeseries.css"

const legendShare = 0.4
const minSelectWidth = 5
const axisFontSize = 12
const noSelect = { left: 0, top: 0, width: 0, height: 0 }
const barSize: [number, number] = [0.6, 64]
const pointSize = 4
const ghostDash = [2, 3]

type Latest = {
  model: Model
  state: PlotState
  entries: TooltipSeries[]
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
}

type PlotConfig = {
  syncKey: string
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
    spanGaps: false,
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
      points: { show: true, size: pointSize, width: 0, fill: stroke, stroke },
    }
  }
  return {
    ...base,
    width: 2,
    ...(fill > 0 ? { fill: fillStyle(stroke, fill) } : {}),
    points: {
      show: false,
      filter: isolatedPoints,
      size: 8,
      fill: stroke,
      stroke,
    },
  }
}

function createPlot(container: HTMLElement, config: PlotConfig): uPlot {
  const { syncKey, host, latest, xAxis, draw } = config
  const timed = xAxis.kind === "time"
  const customPainted = draw !== undefined
  const { model } = latest.current
  const { series, axes, panelAxis, ghostCount } = model
  const { state, width, chartHeight } = latest.current
  const first = 1 + ghostCount
  const colors = readChartColors()
  const font = `${axisFontSize}px ${colors.font}`
  const ghostColor = resolveColor("var(--ghost)")
  const leftSettings = axes.left ?? panelAxis

  const plugins: uPlot.Plugin[] = []
  if (model.tooltip !== "hidden") {
    plugins.push(
      tooltipPlugin(
        host,
        model.tooltip,
        first,
        () => latest.current.entries,
        (x) =>
          xAxis.kind === "time" ? formatDateTime(x * 1000) : xAxis.format(x)
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
  if (draw) plugins.push(draw)

  const valueAxis = (
    key: AxisKey,
    side: 1 | 3,
    settings: AxisSettings,
    grid: boolean,
    show: boolean
  ): uPlot.Axis => ({
    scale: scaleOf[key],
    side,
    show,
    stroke: colors.muted,
    font,
    grid: { show: grid, stroke: colors.grid, width: 1 },
    ticks: { show: false },
    values: (_u, splits) => {
      const decimals = axisDecimals(splits, settings.unit, settings.decimals)
      return splits.map((value) =>
        formatAxisValue(value, settings.unit, decimals)
      )
    },
    size: (u, values) => {
      if (!values) return 48
      u.ctx.font = `${axisFontSize * uPlot.pxRatio}px ${colors.font}`
      const widest = Math.max(
        ...values.map((text) => u.ctx.measureText(text).width)
      )
      return Math.ceil(widest / uPlot.pxRatio) + 14
    },
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
      y: { range: (_u, min, max) => axisRange(leftSettings, min, max) },
      ...(axes.right
        ? {
            y2: {
              range: (_u: uPlot, min: number | null, max: number | null) =>
                axisRange(axes.right!, min, max),
            },
          }
        : {}),
      ...(axes.hidden
        ? {
            y3: {
              range: (_u: uPlot, min: number | null, max: number | null) =>
                axisRange(axes.hidden!, min, max),
            },
          }
        : {}),
    },
    axes: [
      {
        stroke: colors.muted,
        font,
        size: 28,
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
      ...(timed
        ? {
            sync: {
              key: syncKey,
              filters: {
                sub: (type: string) =>
                  type !== "mousedown" &&
                  type !== "mouseup" &&
                  type !== "dblclick",
              },
            },
          }
        : {}),
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
  tooltip: Model["tooltip"],
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
    panelAxis: {
      unit: panel.unit,
      decimals: panel.decimals,
      min: panel.min,
      max: panel.max,
      percent: false,
    },
    ghostCount,
    thresholds: thresholdsFor(series, axes.left, panel),
    tooltip,
  }
}

export type ChartProps = PanelRendererProps & {
  aligned: AlignedFrames
  configs: FieldConfig[]
  xAxis?: XAxis
  draw?: uPlot.Plugin
}

export function TimeseriesChart({
  panel,
  display,
  meta,
  width,
  height,
  syncKey,
  onTimeRangeSelect,
  aligned,
  configs,
  xAxis = timeAxis,
  draw,
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

  const stateKey = `${syncKey}:${panel.id}`
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

  const legendShown = series.length > 1 && options.legend !== "hidden"
  const placement = options.legend === "right" ? "right" : "bottom"
  const hidden = useMemo(
    () => visibleSeries(storedHidden, keys, legendShown),
    [storedHidden, keys, legendShown]
  )
  const cells = useMemo(() => legendCells(series, reducers), [series, reducers])
  const rightLegend = legendShown && placement === "right"
  const textMeasure = useLegendTextMeasure()
  const tableWidth = useMemo(() => {
    if (!rightLegend || reducers.length === 0) return null
    const rem = rootFontSize()
    return legendTableWidth(
      series.map(({ label }) => label),
      reducers.map((reducer) => reducerLabels[reducer]),
      cells,
      textMeasure(rem),
      rem
    )
  }, [rightLegend, series, reducers, cells, textMeasure])
  const legendWidth = rightLegend ? rightLegendWidth(tableWidth, width) : 0
  const chartWidth = width - legendWidth
  const bottomLegend = legendShown && placement === "bottom"
  const chartHeight = Math.max(0, height - (bottomLegend ? legendHeight : 0))
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
      })),
    }
  }, [aligned.xs, ghosts, series, hidden])

  const model = modelFor(
    panel,
    series,
    ghosts.length,
    options.tooltip,
    draw !== undefined
  )

  const latest = useRef<Latest>({
    model,
    state,
    entries,
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
  }, [bottomLegend, reducers.length])

  useLayoutEffect(() => {
    const container = containerRef.current
    const host = hostRef.current
    if (!container || !host || !ready) return
    const plot = createPlot(container, {
      syncKey,
      host,
      latest,
      xAxis,
      draw,
    })
    plotRef.current = plot
    appliedRef.current = latest.current.state
    return () => {
      plot.destroy()
      plotRef.current = null
      appliedRef.current = null
    }
  }, [
    ready,
    signature,
    theme,
    syncKey,
    xAxis,
    draw,
    configsKey,
    displayKey,
    ghosts.length,
    panel.unit,
    panel.decimals,
    panel.min,
    panel.max,
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

  const items: LegendItem[] = series.map((entry) => ({
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

  return (
    <div
      ref={hostRef}
      className={cn(
        "relative flex",
        placement === "right" && legendShown ? "flex-row" : "flex-col"
      )}
      style={{ width, height }}
    >
      <div
        ref={containerRef}
        role="img"
        aria-label={
          xAxis.kind === "time"
            ? `${panel.title}: ${series.length} series, ${formatDateTime(meta.from)} to ${formatDateTime(meta.to)}`
            : `${panel.title}: ${series.length} series`
        }
        data-axes={axisNames.join(" ")}
        data-series={series.length}
        data-ghosts={ghosts.length}
        data-stack={firstStackMode(series)}
        className="ts-chart shrink-0 overflow-hidden"
        style={{ width: chartWidth, height: chartHeight }}
      />
      {legendShown && reducers.length > 0 ? (
        <LegendTable {...legendProps} reducers={reducers} cells={cells} />
      ) : null}
      {legendShown && reducers.length === 0 ? (
        <Legend {...legendProps} />
      ) : null}
    </div>
  )
}

export const TimeseriesPanel: PanelRenderer = (props) => {
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
