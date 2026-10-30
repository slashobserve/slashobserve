import {
  useId,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type PointerEvent,
} from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { readChartColors, resolveColor } from "../colors"
import { cssColorToRgb, darkText, readableText } from "../contrast"
import { blendOver } from "../state-timeline/timeline-layout"
import {
  cellBox,
  cellLabelFontSize,
  cellPaint,
  insetCell,
  isHeatmapKey,
  labelFits,
  stepCell,
  visibleCell,
  type Box,
  type HeatmapCell,
  type HeatmapLook,
  type HeatmapModel,
} from "./heatmap-model"

const focusLineWidth = 2
const haloLineWidth = 1
const keyboardHint =
  "Arrow keys move between cells, Home and End jump to the first and last time bucket, Escape clears the focus."

export type Hover = { row: number; col: number; x: number; y: number }

type Props = {
  model: HeatmapModel
  look: HeatmapLook
  format: (value: number) => string
  width: number
  height: number
  theme: ResolvedTheme
  label: string
  focused: HeatmapCell | null
  onHover: (hover: Hover | null) => void
  onCursorMove: (x: number | null) => void
  onFocusChange: (cell: HeatmapCell | null) => void
}

function strokeInset(
  context: CanvasRenderingContext2D,
  box: ReturnType<typeof cellBox>,
  offset: number,
  lineWidth: number,
  color: string
) {
  const inset = offset + lineWidth / 2
  context.lineWidth = lineWidth
  context.strokeStyle = resolveColor(color)
  context.strokeRect(
    box.left + inset,
    box.top + inset,
    Math.max(0, box.width - 2 * inset),
    Math.max(0, box.height - 2 * inset)
  )
}

function cached<T>(cache: Map<string, T>, key: string, make: () => T): T {
  const hit = cache.get(key)
  if (hit !== undefined) return hit
  const made = make()
  cache.set(key, made)
  return made
}

function fillCell(context: CanvasRenderingContext2D, box: Box, radius: number) {
  const rounded = Math.min(radius, box.width / 2, box.height / 2)
  if (rounded <= 0) {
    context.fillRect(box.left, box.top, box.width, box.height)
    return
  }
  context.beginPath()
  context.roundRect(box.left, box.top, box.width, box.height, rounded)
  context.fill()
}

function draw(
  canvas: HTMLCanvasElement,
  model: HeatmapModel,
  look: HeatmapLook,
  format: (value: number) => string,
  width: number,
  height: number,
  focused: HeatmapCell | null
) {
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.round(width * ratio)
  canvas.height = Math.round(height * ratio)
  const context = canvas.getContext("2d")
  if (!context) return
  context.scale(ratio, ratio)
  const colors = new Map<string, string>()
  const texts = new Map<string, string>()
  const backdrop = cssColorToRgb(resolveColor("var(--background)")) ?? [
    255, 255, 255,
  ]
  const resolved = (color: string) =>
    cached(colors, color, () => resolveColor(color))
  const textOn = (color: string, alpha: number) =>
    cached(texts, `${alpha}|${color}`, () => {
      const rgb = cssColorToRgb(resolved(color))
      return rgb ? readableText(blendOver(rgb, backdrop, alpha)) : darkText
    })
  context.font = `${cellLabelFontSize}px ${readChartColors().font}`
  context.textAlign = "center"
  context.textBaseline = "middle"
  model.cells.forEach((cells, row) => {
    cells.forEach((value, col) => {
      if (!visibleCell(look, value)) return
      const box = insetCell(
        cellBox(model, { row, col }, width, height),
        look.gap
      )
      const paint = cellPaint(look, value)
      context.globalAlpha = paint.alpha
      context.fillStyle = resolved(paint.color)
      fillCell(context, box, look.radius)
      if (look.showValues === "never") return
      const text = format(value)
      if (!labelFits(look.showValues, box, context.measureText(text).width)) {
        return
      }
      context.globalAlpha = 1
      context.fillStyle = textOn(paint.color, paint.alpha)
      context.fillText(
        text,
        box.left + box.width / 2,
        box.top + box.height / 2,
        box.width
      )
    })
  })
  if (focused === null) return
  const box = cellBox(model, focused, width, height)
  context.globalAlpha = 1
  strokeInset(context, box, 0, focusLineWidth, "var(--ring)")
  strokeInset(context, box, focusLineWidth, haloLineWidth, "var(--background)")
}

export function HeatmapCanvas({
  model,
  look,
  format,
  width,
  height,
  theme,
  label,
  focused,
  onHover,
  onCursorMove,
  onFocusChange,
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)
  const hintId = useId()

  useLayoutEffect(() => {
    if (ref.current) {
      draw(ref.current, model, look, format, width, height, focused)
    }
  }, [model, look, format, width, height, theme, focused])

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
    onCursorMove(model.xs[col])
  }

  function onKeyDown(event: KeyboardEvent<HTMLCanvasElement>) {
    if (event.altKey || event.ctrlKey || event.metaKey) return
    if (isHeatmapKey(event.key)) {
      event.preventDefault()
      onHover(null)
      onFocusChange(stepCell(model, focused, event.key))
    } else if (event.key === "Escape" && focused !== null) {
      event.preventDefault()
      onFocusChange(null)
    }
  }

  return (
    <>
      <canvas
        ref={ref}
        tabIndex={0}
        role="group"
        aria-roledescription="heatmap"
        aria-label={label}
        aria-describedby={hintId}
        className="block cursor-crosshair focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
        style={{ width, height }}
        onPointerMove={move}
        onPointerLeave={() => {
          onHover(null)
          onCursorMove(null)
        }}
        onKeyDown={onKeyDown}
        onBlur={() => onFocusChange(null)}
      />
      <span id={hintId} className="sr-only">
        {keyboardHint}
      </span>
    </>
  )
}
