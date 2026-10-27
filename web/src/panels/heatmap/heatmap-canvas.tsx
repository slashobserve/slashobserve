import { useLayoutEffect, useRef, type PointerEvent } from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { resolveColor } from "../colors"
import { cellOpacity, type HeatmapModel } from "./heatmap-model"

export type Hover = { row: number; col: number; x: number; y: number }

type Props = {
  model: HeatmapModel
  width: number
  height: number
  theme: ResolvedTheme
  label: string
  onHover: (hover: Hover | null) => void
}

function edge(index: number, count: number, size: number): number {
  return Math.round((index * size) / count)
}

function draw(
  canvas: HTMLCanvasElement,
  model: HeatmapModel,
  width: number,
  height: number
) {
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.round(width * ratio)
  canvas.height = Math.round(height * ratio)
  const context = canvas.getContext("2d")
  if (!context) return
  context.scale(ratio, ratio)
  context.fillStyle = resolveColor("var(--series-1)")
  const rows = model.rows.length
  const columns = model.xs.length
  model.cells.forEach((cells, row) => {
    const top = edge(rows - row - 1, rows, height)
    const bottom = edge(rows - row, rows, height)
    cells.forEach((value, col) => {
      if (value === null || value <= 0) return
      const left = edge(col, columns, width)
      context.globalAlpha = cellOpacity(value, model.max)
      context.fillRect(
        left,
        top,
        edge(col + 1, columns, width) - left,
        bottom - top
      )
    })
  })
}

export function HeatmapCanvas({
  model,
  width,
  height,
  theme,
  label,
  onHover,
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)

  useLayoutEffect(() => {
    if (ref.current) draw(ref.current, model, width, height)
  }, [model, width, height, theme])

  function move(event: PointerEvent<HTMLCanvasElement>) {
    const box = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - box.left
    const y = event.clientY - box.top
    const col = Math.min(
      model.xs.length - 1,
      Math.max(0, Math.floor((x / width) * model.xs.length))
    )
    const row = Math.min(
      model.rows.length - 1,
      Math.max(
        0,
        model.rows.length - 1 - Math.floor((y / height) * model.rows.length)
      )
    )
    onHover({ row, col, x, y })
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
