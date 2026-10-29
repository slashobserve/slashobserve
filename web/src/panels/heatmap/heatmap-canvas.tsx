import {
  useId,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type PointerEvent,
} from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { resolveColor } from "../colors"
import {
  cellBox,
  cellOpacity,
  edge,
  isHeatmapKey,
  stepCell,
  type HeatmapCell,
  type HeatmapModel,
} from "./heatmap-model"

const focusLineWidth = 2
const haloLineWidth = 1
const keyboardHint =
  "Arrow keys move between cells, Home and End jump to the first and last time bucket, Escape clears the focus."

export type Hover = { row: number; col: number; x: number; y: number }

type Props = {
  model: HeatmapModel
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

function draw(
  canvas: HTMLCanvasElement,
  model: HeatmapModel,
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
  if (focused === null) return
  const box = cellBox(model, focused, width, height)
  context.globalAlpha = 1
  strokeInset(context, box, 0, focusLineWidth, "var(--ring)")
  strokeInset(context, box, focusLineWidth, haloLineWidth, "var(--background)")
}

export function HeatmapCanvas({
  model,
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
    if (ref.current) draw(ref.current, model, width, height, focused)
  }, [model, width, height, theme, focused])

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
