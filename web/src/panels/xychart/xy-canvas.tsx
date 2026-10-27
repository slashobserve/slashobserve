import { useLayoutEffect, useRef, type PointerEvent } from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { readChartColors, resolveColor } from "../colors"
import type { XyModel, XyRange, XyShow, XyTick } from "./xy-model"

type Props = {
  model: XyModel
  width: number
  height: number
  theme: ResolvedTheme
  show: XyShow
  pointSize: number
  xTicks: XyTick[]
  yTicks: XyTick[]
  label: string
  onHover: (position: { x: number; y: number } | null) => void
}

const lineWidth = 1.5

export function toPixelX(value: number, range: XyRange, width: number) {
  return ((value - range.min) / (range.max - range.min)) * width
}

export function toPixelY(value: number, range: XyRange, height: number) {
  return ((range.max - value) / (range.max - range.min)) * height
}

function draw(
  canvas: HTMLCanvasElement,
  props: Omit<Props, "label" | "onHover" | "theme">
) {
  const { model, width, height, show, pointSize, xTicks, yTicks } = props
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.round(width * ratio)
  canvas.height = Math.round(height * ratio)
  const context = canvas.getContext("2d")
  if (!context) return
  context.scale(ratio, ratio)
  context.strokeStyle = readChartColors().grid
  context.lineWidth = 1
  context.beginPath()
  for (const tick of yTicks) {
    const y = Math.round(toPixelY(tick.value, model.y, height)) + 0.5
    context.moveTo(0, y)
    context.lineTo(width, y)
  }
  for (const tick of xTicks) {
    const x = Math.round(toPixelX(tick.value, model.x, width)) + 0.5
    context.moveTo(x, 0)
    context.lineTo(x, height)
  }
  context.stroke()
  const radius = pointSize / 2
  for (const series of model.series) {
    if (show !== "points") {
      context.strokeStyle = resolveColor(series.color)
      context.lineWidth = lineWidth
      context.lineJoin = "round"
      context.beginPath()
      series.points.forEach(([x, y], index) => {
        const px = toPixelX(x, model.x, width)
        const py = toPixelY(y, model.y, height)
        if (index === 0) context.moveTo(px, py)
        else context.lineTo(px, py)
      })
      context.stroke()
    }
    if (show === "lines") continue
    const resolved = new Map<string, string>()
    let current: string | null = null
    for (const [x, y, color] of series.points) {
      if (color !== current) {
        if (current !== null) context.fill()
        current = color
        let fill = resolved.get(color)
        if (fill === undefined) {
          fill = resolveColor(color)
          resolved.set(color, fill)
        }
        context.fillStyle = fill
        context.beginPath()
      }
      const px = toPixelX(x, model.x, width)
      context.moveTo(px + radius, toPixelY(y, model.y, height))
      context.arc(px, toPixelY(y, model.y, height), radius, 0, Math.PI * 2)
    }
    if (current !== null) context.fill()
  }
}

export function XyCanvas({
  model,
  width,
  height,
  theme,
  show,
  pointSize,
  xTicks,
  yTicks,
  label,
  onHover,
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)

  useLayoutEffect(() => {
    if (!ref.current) return
    draw(ref.current, { model, width, height, show, pointSize, xTicks, yTicks })
  }, [model, width, height, theme, show, pointSize, xTicks, yTicks])

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
