import { useLayoutEffect, useRef, type MouseEvent } from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { readChartColors, resolveColor } from "../colors"
import { cssColorToRgb, lightText, readableText } from "../contrast"
import { nodeAt, type FlameProfile, type FlameRange } from "./flame-model"

const rowHeight = 18

const rowGap = 1
const minRectWidth = 1
const minLabelWidth = 24
const labelPad = 4
const dimmedOpacity = 0.3

export type Hover = { index: number; clientX: number; clientY: number }

type Props = {
  profile: FlameProfile
  range: FlameRange
  matches: ReadonlySet<number> | null
  width: number
  label: string
  theme: ResolvedTheme
  onHover: (hover: Hover | null) => void
  onSelect: (index: number) => void
}

function canvasHeight(profile: FlameProfile): number {
  return profile.levels.length * rowHeight
}

function draw(
  canvas: HTMLCanvasElement,
  profile: FlameProfile,
  range: FlameRange,
  matches: ReadonlySet<number> | null,
  width: number
) {
  const height = canvasHeight(profile)
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.round(width * ratio)
  canvas.height = Math.round(height * ratio)
  const context = canvas.getContext("2d")
  if (!context) return
  context.scale(ratio, ratio)
  context.font = `11px ${readChartColors().font}`
  context.textBaseline = "middle"
  const scale = width / (range.end - range.start)
  const fills = new Map<string, { fill: string; text: string }>()
  for (const node of profile.nodes) {
    const left = (node.start - range.start) * scale
    const right = left + node.value * scale
    if (right <= 0 || left >= width) continue
    const x = Math.max(0, left)
    const rectWidth = Math.min(width, right) - x - rowGap
    if (rectWidth < minRectWidth) continue
    let paint = fills.get(node.color)
    if (!paint) {
      const fill = resolveColor(node.color)
      const rgb = cssColorToRgb(fill)
      paint = { fill, text: rgb ? readableText(rgb) : lightText }
      fills.set(node.color, paint)
    }
    const top = node.level * rowHeight
    context.globalAlpha =
      matches === null || matches.has(node.index) ? 1 : dimmedOpacity
    context.fillStyle = paint.fill
    context.fillRect(x, top, rectWidth, rowHeight - rowGap)
    if (rectWidth < minLabelWidth) continue
    context.save()
    context.beginPath()
    context.rect(x, top, rectWidth, rowHeight - rowGap)
    context.clip()
    context.fillStyle = paint.text
    context.fillText(node.label, x + labelPad, top + (rowHeight - rowGap) / 2)
    context.restore()
  }
}

export function FlameCanvas({
  profile,
  range,
  matches,
  width,
  label,
  theme,
  onHover,
  onSelect,
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)

  useLayoutEffect(() => {
    if (ref.current) draw(ref.current, profile, range, matches, width)
  }, [profile, range, matches, width, theme])

  function nodeUnder(event: MouseEvent<HTMLCanvasElement>): number | null {
    const box = event.currentTarget.getBoundingClientRect()
    const level = Math.floor((event.clientY - box.top) / rowHeight)
    const fraction = (event.clientX - box.left) / width
    return nodeAt(profile, range, level, fraction)
  }

  function move(event: MouseEvent<HTMLCanvasElement>) {
    const index = nodeUnder(event)
    onHover(
      index === null
        ? null
        : { index, clientX: event.clientX, clientY: event.clientY }
    )
  }

  function select(event: MouseEvent<HTMLCanvasElement>) {
    const index = nodeUnder(event)
    if (index !== null) onSelect(index)
  }

  return (
    <canvas
      ref={ref}
      role="img"
      aria-label={label}
      className="block cursor-pointer"
      style={{ width, height: canvasHeight(profile) }}
      onPointerMove={move}
      onPointerLeave={() => onHover(null)}
      onClick={select}
    />
  )
}
