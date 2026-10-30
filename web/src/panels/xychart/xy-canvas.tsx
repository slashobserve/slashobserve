import { useLayoutEffect, useRef, type PointerEvent } from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { readChartColors, resolveColor } from "../colors"
import {
  axisFraction,
  type XyAxis,
  type XyModel,
  type XySeries,
  type XyShow,
  type XyTick,
} from "./xy-model"

type Props = {
  model: XyModel
  width: number
  height: number
  theme: ResolvedTheme
  show: XyShow
  xTicks: XyTick[]
  yTicks: XyTick[]
  label: string
  onHover: (position: { x: number; y: number } | null) => void
}

export function toPixelX(model: XyModel, value: number, width: number) {
  return axisFraction(model.x, value) * width
}

export function toPixelY(model: XyModel, value: number, height: number) {
  return (1 - axisFraction(model.y, value)) * height
}

function drawGrid(
  context: CanvasRenderingContext2D,
  props: Pick<Props, "model" | "width" | "height" | "xTicks" | "yTicks">
) {
  const { model, width, height, xTicks, yTicks } = props
  context.strokeStyle = readChartColors().grid
  context.lineWidth = 1
  context.beginPath()
  if (model.y.settings.grid && model.y.settings.side !== "hidden") {
    for (const tick of yTicks) {
      const y = Math.round(toPixelY(model, tick.value, height)) + 0.5
      context.moveTo(0, y)
      context.lineTo(width, y)
    }
  }
  if (model.x.settings.grid && model.x.settings.side !== "hidden") {
    for (const tick of xTicks) {
      const x = Math.round(toPixelX(model, tick.value, width)) + 0.5
      context.moveTo(x, 0)
      context.lineTo(x, height)
    }
  }
  context.stroke()
}

function drawBorder(
  context: CanvasRenderingContext2D,
  axis: XyAxis,
  from: [number, number],
  to: [number, number]
) {
  if (!axis.settings.border || axis.settings.side === "hidden") return
  context.strokeStyle = axis.color
    ? resolveColor(axis.color)
    : readChartColors().muted
  context.lineWidth = 1
  context.beginPath()
  context.moveTo(from[0], from[1])
  context.lineTo(to[0], to[1])
  context.stroke()
}

function drawLine(
  context: CanvasRenderingContext2D,
  model: XyModel,
  series: XySeries,
  size: { width: number; height: number }
) {
  context.strokeStyle = resolveColor(series.color)
  context.lineWidth = series.lineWidth
  context.lineJoin = "round"
  context.setLineDash(series.dash)
  context.beginPath()
  series.points.forEach(([x, y], index) => {
    const px = toPixelX(model, x, size.width)
    const py = toPixelY(model, y, size.height)
    if (index === 0) context.moveTo(px, py)
    else context.lineTo(px, py)
  })
  context.stroke()
  context.setLineDash([])
}

function drawPoints(
  context: CanvasRenderingContext2D,
  model: XyModel,
  series: XySeries,
  size: { width: number; height: number }
) {
  const resolved = new Map<string, string>()
  let current: string | null = null
  const flush = () => {
    if (current === null) return
    context.globalAlpha = series.fillAlpha
    context.fill()
    if (series.strokeWidth > 0) {
      context.globalAlpha = 1
      context.strokeStyle = context.fillStyle
      context.lineWidth = series.strokeWidth
      context.stroke()
    }
    context.globalAlpha = 1
  }
  for (const [x, y, color, diameter] of series.points) {
    if (color !== current) {
      flush()
      current = color
      let fill = resolved.get(color)
      if (fill === undefined) {
        fill = resolveColor(color)
        resolved.set(color, fill)
      }
      context.fillStyle = fill
      context.beginPath()
    }
    const px = toPixelX(model, x, size.width)
    const py = toPixelY(model, y, size.height)
    const radius = diameter / 2
    if (series.square) {
      context.rect(px - radius, py - radius, diameter, diameter)
    } else {
      context.moveTo(px + radius, py)
      context.arc(px, py, radius, 0, Math.PI * 2)
    }
  }
  flush()
}

function draw(
  canvas: HTMLCanvasElement,
  props: Omit<Props, "label" | "onHover" | "theme">
) {
  const { model, width, height, show } = props
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.round(width * ratio)
  canvas.height = Math.round(height * ratio)
  const context = canvas.getContext("2d")
  if (!context) return
  context.scale(ratio, ratio)
  drawGrid(context, props)
  drawBorder(
    context,
    model.y,
    model.y.settings.side === "end" ? [width, 0] : [0, 0],
    model.y.settings.side === "end" ? [width, height] : [0, height]
  )
  drawBorder(context, model.x, [0, height], [width, height])
  const size = { width, height }
  for (const series of model.series) {
    if (show !== "points") drawLine(context, model, series, size)
    if (show !== "lines") drawPoints(context, model, series, size)
  }
}

export function XyCanvas({
  model,
  width,
  height,
  theme,
  show,
  xTicks,
  yTicks,
  label,
  onHover,
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)

  useLayoutEffect(() => {
    if (!ref.current) return
    draw(ref.current, { model, width, height, show, xTicks, yTicks })
  }, [model, width, height, theme, show, xTicks, yTicks])

  function move(event: PointerEvent<HTMLCanvasElement>) {
    const box = event.currentTarget.getBoundingClientRect()
    onHover({ x: event.clientX - box.left, y: event.clientY - box.top })
  }

  return (
    <canvas
      ref={ref}
      role="img"
      aria-label={label}
      className="block cursor-crosshair"
      style={{ width, height }}
      onPointerMove={move}
      onPointerLeave={() => onHover(null)}
    />
  )
}
