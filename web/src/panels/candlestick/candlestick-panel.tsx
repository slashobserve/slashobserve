import { useMemo } from "react"
import uPlot from "uplot"

import { stableJson } from "@/lib/json"
import { cn } from "@/lib/utils"

import { resolveColor } from "../colors"
import { dataRenderers } from "../index"
import { alignFrames } from "../timeseries/align"
import type { Cell } from "../timeseries/stack"
import { LegendSwatch } from "../timeseries/legend"
import { TimeseriesChart } from "../timeseries/timeseries-panel"
import type { FieldConfig } from "../field-config"
import type { PreparedFrame } from "../transforms"
import type { PanelRenderer } from "../types"
import { Unsupported } from "../unsupported"
import {
  bodyWidth,
  candleOptions,
  directionColors,
  directions,
  minSpacing,
  roleLabels,
  rowColors,
  selectCandles,
  volumeHeight,
  type CandleColumn,
  type CandleMode,
  type CandleOptions,
  type CandleSelection,
  type CandleRole,
  type CandleStyle,
  type ColorStrategy,
} from "./candles"

const legendBottomHeight = 28
const legendRightWidth = 96

type Paint = {
  mode: CandleMode
  candleStyle: CandleStyle
  colorStrategy: ColorStrategy
  upColor: string
  downColor: string
  columns: Partial<Record<CandleRole, number>>
}

function candlesPlugin(paint: Paint): uPlot.Plugin {
  const { mode, candleStyle, colorStrategy, columns } = paint
  return {
    hooks: {
      draw: (u) => {
        const { ctx } = u
        const { left, top, width, height } = u.bbox
        const ratio = uPlot.pxRatio
        const stroke = Math.max(1, Math.round(ratio))
        const xs = u.data[0] as number[]
        const column = (
          role: CandleRole
        ): readonly (number | null | undefined)[] =>
          columns[role] === undefined ? [] : (u.data[columns[role]] as Cell[])
        const open = column("open")
        const high = column("high")
        const low = column("low")
        const close = column("close")
        const volume = column("volume")
        const centers = xs.map((x) => u.valToPos(x, "x", true))
        const size = bodyWidth(minSpacing(centers), ratio)
        const half = Math.round(size / 2)
        const resolved = new Map<string, string>()
        const fills = rowColors(open, close, xs.length, paint).map((color) => {
          const known = resolved.get(color) ?? resolveColor(color)
          resolved.set(color, known)
          return known
        })
        const moves = directions(open, close, colorStrategy)
        const at = (value: number) => Math.round(u.valToPos(value, "y", true))

        ctx.save()
        ctx.beginPath()
        ctx.rect(left, top, width, height)
        ctx.clip()

        if (mode !== "candles") {
          const maxVolume = volume.reduce<number>(
            (best, value) =>
              typeof value === "number" && value > best ? value : best,
            0
          )
          xs.forEach((_x, row) => {
            const bar = volumeHeight(volume[row], maxVolume, height)
            if (bar <= 0) return
            ctx.fillStyle = fills[row]
            ctx.fillRect(
              Math.round(centers[row]) - half,
              Math.round(top + height - bar),
              half * 2,
              Math.max(stroke, Math.round(bar))
            )
          })
        }

        if (mode !== "volume") {
          xs.forEach((_x, row) => {
            const direction = moves[row]
            const hi = high[row]
            const lo = low[row]
            const opened = open[row]
            const closed = close[row]
            if (
              !direction ||
              typeof hi !== "number" ||
              typeof lo !== "number" ||
              typeof opened !== "number" ||
              typeof closed !== "number"
            ) {
              return
            }
            const x = Math.round(centers[row])
            const yHigh = at(hi)
            const yLow = at(lo)
            const yOpen = at(opened)
            const yClose = at(closed)
            ctx.fillStyle = fills[row]
            ctx.fillRect(
              x - Math.floor(stroke / 2),
              yHigh,
              stroke,
              Math.max(stroke, yLow - yHigh)
            )
            if (candleStyle === "candles") {
              ctx.fillRect(
                x - half,
                Math.min(yOpen, yClose),
                half * 2,
                Math.max(stroke, Math.abs(yClose - yOpen))
              )
            } else {
              ctx.fillRect(x - half, yOpen, half, stroke)
              ctx.fillRect(x, yClose, half, stroke)
            }
          })
        }
        ctx.restore()
      },
    },
  }
}

function paintKeyOf(options: CandleOptions, columns: CandleColumn[]): string {
  return stableJson({
    mode: options.mode,
    candleStyle: options.candleStyle,
    colorStrategy: options.colorStrategy,
    upColor: options.upColor,
    downColor: options.downColor,
    columns: Object.fromEntries(
      columns.map(({ role }, index) => [role, index + 1])
    ),
  })
}

function candlesPluginFor(paintKey: string): uPlot.Plugin {
  return candlesPlugin(JSON.parse(paintKey) as Paint)
}

function selectCandlesFor(
  frames: PreparedFrame[],
  optionsKey: string
): CandleSelection {
  return selectCandles(frames, JSON.parse(optionsKey) as CandleOptions)
}

function priceConfig(config: FieldConfig, role: CandleRole): FieldConfig {
  return {
    ...config,
    style: "line",
    fill: 0,
    stack: "none",
    hidden: false,
    axis: role === "volume" ? "hidden" : "left",
  }
}

export const CandlestickPanel: PanelRenderer = (props) => {
  const { frames, fields, display, width, height } = props
  const options = useMemo(() => candleOptions(display), [display])
  const optionsKey = useMemo(() => stableJson(options), [options])
  const selection = useMemo(
    () => selectCandlesFor(frames, optionsKey),
    [frames, optionsKey]
  )
  const source = selection.reason === null ? selection.frame : null
  const columns = useMemo(
    () => (selection.reason === null ? selection.columns : []),
    [selection]
  )
  const aligned = useMemo(() => {
    const labels = new Map(
      columns.map(({ role, field }) => [field, roleLabels[role]])
    )
    return source
      ? alignFrames(
          [source],
          (_frame, field) => labels.get(field) ?? field.name
        )
      : null
  }, [source, columns])
  const configs = useMemo(() => {
    if (!aligned) return []
    const roles = new Map(columns.map(({ role, field }) => [field, role]))
    return aligned.series.map(({ frame, field }) =>
      priceConfig(fields.configFor(frame, field), roles.get(field) ?? "close")
    )
  }, [aligned, columns, fields])
  const rowColor = useMemo(() => {
    if (!aligned) return undefined
    const values = (role: CandleRole) => {
      const index = columns.findIndex((column) => column.role === role)
      return index < 0 ? [] : aligned.series[index].values
    }
    const colors = rowColors(
      values("open"),
      values("close"),
      aligned.xs.length,
      options
    )
    return (row: number) => colors[row]
  }, [aligned, columns, options])
  const paintKey = useMemo(
    () => paintKeyOf(options, columns),
    [options, columns]
  )
  const draw = useMemo(() => candlesPluginFor(paintKey), [paintKey])

  if (selection.reason !== null) {
    return (
      <p className="m-0 p-3 text-xs text-muted-foreground">
        {selection.reason}
      </p>
    )
  }
  if (!aligned || aligned.series.length === 0 || aligned.xs.length === 0) {
    const Table = dataRenderers.table
    return Table ? <Table {...props} /> : <Unsupported />
  }

  const shades = directionColors(options)
  const legendShown = options.legend !== "hidden"
  const right = options.legend === "right"
  const chartWidth = width - (legendShown && right ? legendRightWidth : 0)
  const chartHeight = height - (legendShown && !right ? legendBottomHeight : 0)
  const chartDisplay = {
    ...display,
    legend: "hidden",
    legendValues: [],
    tooltip: options.tooltip === "hidden" ? "hidden" : "all",
  }

  return (
    <div
      className={cn("flex", right ? "flex-row" : "flex-col")}
      style={{ width, height }}
    >
      <TimeseriesChart
        {...props}
        display={chartDisplay}
        width={chartWidth}
        height={chartHeight}
        aligned={aligned}
        configs={configs}
        draw={draw}
        rowColor={rowColor}
      />
      {legendShown ? (
        <ul
          aria-label="Legend"
          className={cn(
            "m-0 flex list-none items-center gap-3 p-0 text-xs text-muted-foreground",
            right
              ? "flex-col items-start justify-center px-2"
              : "justify-center"
          )}
          style={
            right ? { width: legendRightWidth } : { height: legendBottomHeight }
          }
        >
          {(["up", "down"] as const).map((direction) => (
            <li key={direction} className="flex items-center gap-1.5">
              <LegendSwatch
                color={shades[direction]}
                line="solid"
                style="bars"
              />
              {direction === "up" ? "Up" : "Down"}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

export default CandlestickPanel
