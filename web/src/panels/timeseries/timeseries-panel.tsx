import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react"
import uPlot from "uplot"
import "uplot/dist/uPlot.min.css"

import type { PanelDataMeta } from "@/dashboards/queries"
import { formatDateTime } from "@/lib/time"
import { useTheme } from "@/lib/theme"
import {
  axisDecimals,
  commonDecimals,
  formatAxisValue,
  formatValue,
} from "@/lib/units"
import type { DataPanel } from "@/spec"

import {
  assignSlots,
  readChartColors,
  resolveColor,
  styleForSlot,
  type SeriesStyle,
} from "../colors"
import { TablePanel } from "../table-panel"
import { thresholdColor } from "../thresholds"
import type { PanelRenderer, PanelRendererProps } from "../types"
import { alignFrames, type AlignedFrames } from "./align"
import { Legend } from "./legend"
import { thresholdsPlugin, tooltipPlugin } from "./plugins"
import { isolatedPoints } from "./points"
import { emptySeriesState, useSeriesStore, visibleSeries } from "./series-state"
import "./timeseries.css"

const legendShare = 0.4
const minSelectWidth = 5
const axisFontSize = 12
const noSelect = { left: 0, top: 0, width: 0, height: 0 }

type Latest = {
  aligned: AlignedFrames
  styles: SeriesStyle[]
  hidden: ReadonlySet<string>
  meta: PanelDataMeta
  width: number
  chartHeight: number
  onTimeRangeSelect: PanelRendererProps["onTimeRangeSelect"]
}

type PlotConfig = {
  panel: Pick<DataPanel, "unit" | "decimals" | "min" | "max" | "thresholds">
  syncKey: string
  host: HTMLElement
  latest: { current: Latest }
}

function plotData(aligned: AlignedFrames): uPlot.AlignedData {
  return [aligned.xs, ...aligned.series.map((series) => series.values)]
}

function autoRange(min: number | null, max: number | null): [number, number] {
  if (min == null || max == null || !Number.isFinite(min + max)) return [0, 1]
  if (min >= 0) {
    if (max <= 0) return [0, 1]
    return [0, max + max * 0.05]
  }
  const pad = max === min ? Math.abs(min) * 0.05 : (max - min) * 0.05
  return [min - pad, max + pad]
}

function yRange(
  panel: PlotConfig["panel"],
  min: number | null,
  max: number | null
): [number, number] {
  const [autoMin, autoMax] = autoRange(min, max)
  const lo = panel.min ?? autoMin
  let hi = panel.max ?? autoMax
  if (hi <= lo) hi = lo + 1
  return [lo, hi]
}

const dayMs = 86_400_000
const tickEdgeGap = 24

function timeLabels(splits: (number | null)[], spanSeconds: number): string[] {
  const spanMs = spanSeconds * 1000
  return splits.map((seconds) => {
    if (seconds === null) return ""
    const text = formatDateTime(seconds * 1000)
    if (spanMs > 7 * dayMs) return text.slice(5, 10)
    if (spanMs > dayMs) return text.slice(5, 16)
    return spanMs < 300_000 ? text.slice(11, 19) : text.slice(11, 16)
  })
}

const dropEdgeTicks: uPlot.Axis.Filter = (u, splits) =>
  splits.map((seconds) => {
    const x = u.valToPos(seconds, "x")
    return x < tickEdgeGap || x > u.bbox.width / uPlot.pxRatio - tickEdgeGap
      ? null
      : seconds
  })

function createPlot(container: HTMLElement, config: PlotConfig): uPlot {
  const { panel, syncKey, host, latest } = config
  const { aligned, styles, hidden, width, chartHeight } = latest.current
  const colors = readChartColors()
  const font = `${axisFontSize}px ${colors.font}`
  const lineColors = styles.map((style) => resolveColor(style.color))

  const plugins: uPlot.Plugin[] = [
    tooltipPlugin(
      host,
      () =>
        aligned.series.map((series, index) => ({
          label: series.label,
          color: styles[index].color,
          line: styles[index].line,
        })),
      (values) => {
        const decimals = commonDecimals(values, panel.unit, panel.decimals)
        return values.map((value) => formatValue(value, panel.unit, decimals))
      }
    ),
  ]
  if (panel.thresholds) {
    plugins.push(
      thresholdsPlugin(panel.thresholds, (color) =>
        resolveColor(thresholdColor(color))
      )
    )
  }

  const options: uPlot.Options = {
    width,
    height: chartHeight,
    padding: [8, 12, 0, 0],
    legend: { show: false },
    scales: {
      x: {
        time: true,
        range: (_u, dataMin, dataMax) => {
          const { meta } = latest.current
          const from = Date.parse(meta.from) / 1000
          const to = Date.parse(meta.to) / 1000
          if (Number.isFinite(from) && Number.isFinite(to) && to > from) {
            return [from, to]
          }
          return dataMin === dataMax
            ? [dataMin - 1, dataMax + 1]
            : [dataMin, dataMax]
        },
      },
      y: { range: (_u, min, max) => yRange(panel, min, max) },
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
          timeLabels(splits, (u.scales.x.max ?? 0) - (u.scales.x.min ?? 0)),
      },
      {
        stroke: colors.muted,
        font,
        grid: { stroke: colors.grid, width: 1 },
        ticks: { show: false },
        values: (_u, splits) => {
          const decimals = axisDecimals(splits, panel.unit, panel.decimals)
          return splits.map((value) =>
            formatAxisValue(value, panel.unit, decimals)
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
      },
    ],
    series: [
      {},
      ...aligned.series.map((series, index) => ({
        label: series.label,
        show: !hidden.has(series.key),
        stroke: lineColors[index],
        dash: styles[index].dash,
        width: 2,
        spanGaps: false,
        points: {
          show: false,
          filter: isolatedPoints,
          size: 8,
          fill: lineColors[index],
          stroke: lineColors[index],
        },
      })),
    ],
    cursor: {
      sync: {
        key: syncKey,
        filters: {
          sub: (type) =>
            type !== "mousedown" && type !== "mouseup" && type !== "dblclick",
        },
      },
      drag: { x: true, y: false, setScale: false },
      points: { size: 6 },
    },
    hooks: {
      setSelect: [
        (u) => {
          const { left, width: selected } = u.select
          if (selected >= minSelectWidth) {
            const from = u.posToVal(left, "x") * 1000
            const to = u.posToVal(left + selected, "x") * 1000
            latest.current.onTimeRangeSelect(Math.round(from), Math.round(to))
          }
          u.setSelect(noSelect, false)
        },
      ],
    },
    plugins,
  }

  return new uPlot(options, plotData(aligned), container)
}

function TimeseriesChart({
  panel,
  meta,
  width,
  height,
  syncKey,
  onTimeRangeSelect,
  aligned,
}: PanelRendererProps & { aligned: AlignedFrames }) {
  const { resolved: theme } = useTheme()
  const hostRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const legendRef = useRef<HTMLDivElement>(null)
  const plotRef = useRef<uPlot | null>(null)
  const appliedRef = useRef<AlignedFrames | null>(null)
  const [legendHeight, setLegendHeight] = useState(0)

  const stateKey = `${syncKey}:${panel.id}`
  const stored = useSeriesStore((store) => store.panels[stateKey])
  const commitKeys = useSeriesStore((store) => store.commit)
  const toggleKey = useSeriesStore((store) => store.toggle)
  const { slots, hidden: storedHidden } = stored ?? emptySeriesState

  const keys = useMemo(
    () => aligned.series.map((series) => series.key),
    [aligned.series]
  )
  const styles = useMemo(() => {
    const assigned = assignSlots(slots, keys)
    return keys.map((key) => styleForSlot(assigned.get(key) ?? 0))
  }, [slots, keys])

  const showLegend = aligned.series.length > 1
  const hidden = useMemo(
    () => visibleSeries(storedHidden, keys, showLegend),
    [storedHidden, keys, showLegend]
  )
  const chartHeight = Math.max(0, height - (showLegend ? legendHeight : 0))
  const ready = width > 0 && chartHeight > 0
  const signature = aligned.series
    .map((series) => `${series.key}\u0001${series.label}`)
    .join("\u0000")
  const thresholdsKey = JSON.stringify(panel.thresholds ?? null)

  const latest = useRef<Latest>({
    aligned,
    styles,
    hidden,
    meta,
    width,
    chartHeight,
    onTimeRangeSelect,
  })
  useLayoutEffect(() => {
    latest.current = {
      aligned,
      styles,
      hidden,
      meta,
      width,
      chartHeight,
      onTimeRangeSelect,
    }
  })

  useLayoutEffect(() => {
    commitKeys(stateKey, keys)
  }, [commitKeys, stateKey, keys])

  useLayoutEffect(() => {
    const element = legendRef.current
    if (!element) return
    const observer = new ResizeObserver(() =>
      setLegendHeight(Math.ceil(element.getBoundingClientRect().height))
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [showLegend])

  useLayoutEffect(() => {
    const container = containerRef.current
    const host = hostRef.current
    if (!container || !host || !ready) return
    const plot = createPlot(container, {
      panel: {
        unit: panel.unit,
        decimals: panel.decimals,
        min: panel.min,
        max: panel.max,
        thresholds: panel.thresholds,
      },
      syncKey,
      host,
      latest,
    })
    plotRef.current = plot
    appliedRef.current = latest.current.aligned
    return () => {
      plot.destroy()
      plotRef.current = null
      appliedRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- panel.thresholds is tracked through thresholdsKey so a refetched but equal Spec never re-creates the plot
  }, [
    ready,
    signature,
    theme,
    syncKey,
    panel.unit,
    panel.decimals,
    panel.min,
    panel.max,
    thresholdsKey,
  ])

  useLayoutEffect(() => {
    const plot = plotRef.current
    if (!plot || appliedRef.current === aligned) return
    plot.setData(plotData(aligned))
    appliedRef.current = aligned
  }, [aligned])

  useLayoutEffect(() => {
    const plot = plotRef.current
    if (!plot || !ready) return
    if (plot.width !== width || plot.height !== chartHeight) {
      plot.setSize({ width, height: chartHeight })
    }
  }, [ready, width, chartHeight])

  useLayoutEffect(() => {
    const plot = plotRef.current
    if (!plot) return
    aligned.series.forEach((series, index) => {
      const show = !hidden.has(series.key)
      if (plot.series[index + 1]?.show !== show) {
        plot.setSeries(index + 1, { show })
      }
    })
  }, [hidden, aligned])

  const toggle = useCallback(
    (key: string) => toggleKey(stateKey, key),
    [toggleKey, stateKey]
  )

  return (
    <div
      ref={hostRef}
      className="relative flex flex-col"
      style={{ width, height }}
    >
      <div
        ref={containerRef}
        role="img"
        aria-label={`${panel.title}: ${aligned.series.length} series, ${formatDateTime(meta.from)} to ${formatDateTime(meta.to)}`}
        className="ts-chart shrink-0 overflow-hidden"
        style={{ width, height: chartHeight }}
      />
      {showLegend ? (
        <Legend
          ref={legendRef}
          items={aligned.series.map((series, index) => ({
            key: series.key,
            label: series.label,
            style: styles[index],
          }))}
          hidden={hidden}
          available={height * legendShare}
          onToggle={toggle}
        />
      ) : null}
    </div>
  )
}

export const TimeseriesPanel: PanelRenderer = (props) => {
  const { frames, panel } = props
  const legend = panel.queries[0]?.legend
  const aligned = useMemo(() => alignFrames(frames, legend), [frames, legend])
  if (aligned.series.length === 0 || aligned.xs.length === 0) {
    return <TablePanel {...props} />
  }
  return <TimeseriesChart {...props} aligned={aligned} />
}
