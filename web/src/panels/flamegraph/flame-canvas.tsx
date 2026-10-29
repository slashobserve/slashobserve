import {
  useId,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type MouseEvent,
} from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { readChartColors, resolveColor } from "../colors"
import { cssColorToRgb, lightText, readableText } from "../contrast"
import {
  isFlameKey,
  nodeAt,
  stepFocus,
  type FlameProfile,
  type FlameRange,
} from "./flame-model"

const rowHeight = 18

const rowGap = 1
const minRectWidth = 1
const minLabelWidth = 24
const labelPad = 4
const dimmedOpacity = 0.3
const focusLineWidth = 2
const keyboardHint =
  "Arrow keys move between frames, Enter zooms, Escape resets."

export type Hover = { index: number; clientX: number; clientY: number }

type Props = {
  profile: FlameProfile
  range: FlameRange
  matches: ReadonlySet<number> | null
  width: number
  label: string
  theme: ResolvedTheme
  focused: number | null
  onHover: (hover: Hover | null) => void
  onSelect: (index: number) => void
  onFocusChange: (index: number | null) => void
  onReset: (() => void) | null
}

function canvasHeight(profile: FlameProfile): number {
  return profile.levels.length * rowHeight
}

function draw(
  canvas: HTMLCanvasElement,
  profile: FlameProfile,
  range: FlameRange,
  matches: ReadonlySet<number> | null,
  width: number,
  focused: number | null
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
  let focusBox: { x: number; rectWidth: number; level: number } | null = null
  for (const node of profile.nodes) {
    const left = (node.start - range.start) * scale
    const right = left + node.value * scale
    if (right <= 0 || left >= width) continue
    const x = Math.max(0, left)
    const rectWidth = Math.min(width, right) - x - rowGap
    if (node.index === focused) focusBox = { x, rectWidth, level: node.level }
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
  if (focusBox !== null) {
    const inset = focusLineWidth / 2
    context.globalAlpha = 1
    context.lineWidth = focusLineWidth
    context.strokeStyle = resolveColor("var(--ring)")
    context.strokeRect(
      focusBox.x + inset,
      focusBox.level * rowHeight + inset,
      Math.max(0, focusBox.rectWidth - focusLineWidth),
      rowHeight - rowGap - focusLineWidth
    )
  }
}

export function FlameCanvas({
  profile,
  range,
  matches,
  width,
  label,
  theme,
  focused,
  onHover,
  onSelect,
  onFocusChange,
  onReset,
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)
  const hintId = useId()

  useLayoutEffect(() => {
    if (ref.current) {
      draw(ref.current, profile, range, matches, width, focused)
    }
  }, [profile, range, matches, width, theme, focused])

  useLayoutEffect(() => {
    const box = ref.current?.parentElement
    const node = focused === null ? undefined : profile.nodes[focused]
    if (!box || !node) return
    const top = node.level * rowHeight
    if (top < box.scrollTop) box.scrollTo({ top })
    else if (top + rowHeight > box.scrollTop + box.clientHeight) {
      box.scrollTo({ top: top + rowHeight - box.clientHeight })
    }
  }, [focused, profile])

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

  function onKeyDown(event: KeyboardEvent<HTMLCanvasElement>) {
    if (event.altKey || event.ctrlKey || event.metaKey) return
    if (isFlameKey(event.key)) {
      event.preventDefault()
      onHover(null)
      onFocusChange(stepFocus(profile, focused, event.key))
    } else if (event.key === "Enter" || event.key === " ") {
      if (focused === null) return
      event.preventDefault()
      onSelect(focused)
    } else if (event.key === "Escape" && onReset) {
      event.preventDefault()
      onReset()
    }
  }

  return (
    <>
      <canvas
        ref={ref}
        tabIndex={0}
        role="group"
        aria-roledescription="flame graph"
        aria-label={label}
        aria-describedby={hintId}
        className="block cursor-pointer focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
        style={{ width, height: canvasHeight(profile) }}
        onPointerMove={move}
        onPointerLeave={() => onHover(null)}
        onClick={select}
        onKeyDown={onKeyDown}
      />
      <span id={hintId} className="sr-only">
        {keyboardHint}
      </span>
    </>
  )
}
