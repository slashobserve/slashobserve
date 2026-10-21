import uPlot from "uplot"

import { formatDateTime } from "@/lib/time"
import type { Threshold } from "@/spec"

import type { LineStyle } from "../colors"
import type { FieldConfig } from "../field-config"
import { formatTogether, type DrawStyle } from "./series-config"
import type { Cell } from "./stack"

const maxTooltipRows = 10
const tooltipGap = 12
const tooltipEdge = 4

export type TooltipSeries = {
  label: string
  color: string
  line: LineStyle
  style: DrawStyle
  scale: string
  config: FieldConfig
  values: Cell[]
  stacked: boolean
  below: number | null
}

type Row = { index: number; value: number }

function swatch(color: string, line: LineStyle, style: DrawStyle): HTMLElement {
  const element = document.createElement("span")
  element.className = "series-swatch"
  element.dataset.line = line
  element.dataset.shape = style
  element.style.color = color
  return element
}

function tooltipRow(entry: TooltipSeries, text: string): HTMLElement {
  const line = document.createElement("div")
  line.className = "ts-tooltip-row"
  const label = document.createElement("span")
  label.className = "ts-tooltip-label"
  label.textContent = entry.label
  const value = document.createElement("span")
  value.className = "ts-tooltip-value"
  value.textContent = text
  line.append(swatch(entry.color, entry.line, entry.style), label, value)
  return line
}

function moreLine(count: number): HTMLElement {
  const more = document.createElement("div")
  more.className = "ts-tooltip-more"
  more.textContent = `${count} more`
  return more
}

function distanceTo(top: number, a: number, b: number): number {
  const lo = Math.min(a, b)
  const hi = Math.max(a, b)
  return top < lo ? lo - top : top > hi ? top - hi : 0
}

function nearestRow(
  u: uPlot,
  rows: Row[],
  entries: TooltipSeries[],
  first: number,
  idx: number,
  top: number
): Row {
  let best = rows[0]
  let bestDistance = Infinity
  for (const row of rows) {
    const entry = entries[row.index]
    const drawn = u.data[first + row.index][idx] as number
    const lowerAt = entry.below === null ? null : first + entry.below
    const base =
      lowerAt !== null ? (u.data[lowerAt][idx] ?? 0) : entry.stacked ? 0 : drawn
    const distance = distanceTo(
      top,
      u.valToPos(drawn, entry.scale),
      u.valToPos(base, entry.scale)
    )
    if (distance < bestDistance) {
      best = row
      bestDistance = distance
    }
  }
  return best
}

export function tooltipPlugin(
  host: HTMLElement,
  mode: "single" | "all",
  first: number,
  series: () => TooltipSeries[]
): uPlot.Plugin {
  let tooltip: HTMLDivElement | null = null
  let plot: uPlot | null = null
  let observer: ResizeObserver | null = null
  let geometry = { originX: 0, originY: 0, width: 0, height: 0 }

  function hide() {
    if (tooltip) tooltip.hidden = true
  }

  function measure() {
    if (!plot) return
    const hostRect = host.getBoundingClientRect()
    const overRect = plot.over.getBoundingClientRect()
    geometry = {
      originX: overRect.left - hostRect.left,
      originY: overRect.top - hostRect.top,
      width: hostRect.width,
      height: hostRect.height,
    }
  }

  return {
    hooks: {
      init: (u) => {
        plot = u
        observer = new ResizeObserver(measure)
        observer.observe(host)
        tooltip = document.createElement("div")
        tooltip.className = "ts-tooltip"
        tooltip.dataset.slot = "chart-tooltip"
        tooltip.setAttribute("aria-hidden", "true")
        tooltip.hidden = true
        host.appendChild(tooltip)
      },
      ready: measure,
      setSize: measure,
      destroy: () => {
        observer?.disconnect()
        observer = null
        plot = null
        tooltip?.remove()
        tooltip = null
      },
      setCursor: (u) => {
        const idx = u.cursor.idx
        const left = u.cursor.left ?? -1
        if (!tooltip || idx == null || left < 0) return hide()

        const meta = series()
        const rows: Row[] = []
        meta.forEach((entry, index) => {
          const value = entry.values[idx]
          if (u.series[first + index]?.show && typeof value === "number") {
            rows.push({ index, value })
          }
        })
        if (rows.length === 0) return hide()
        const hovered = u.over.matches(":hover")
        const candidates =
          mode === "single"
            ? [
                nearestRow(
                  u,
                  rows,
                  meta,
                  first,
                  idx,
                  hovered ? (u.cursor.top ?? 0) : 0
                ),
              ]
            : rows.sort((a, b) => b.value - a.value).slice(0, maxTooltipRows)
        const shown = mode === "single" ? 1 : rows.length
        const texts = formatTogether(
          candidates.map((row) => ({
            value: row.value,
            config: meta[row.index].config,
          }))
        )

        const heading = document.createElement("div")
        heading.className = "ts-tooltip-time"
        heading.textContent = formatDateTime(u.data[0][idx] * 1000)

        const render = (count: number, withMore = false) => {
          const children: HTMLElement[] = [heading]
          for (let position = 0; position < count; position++) {
            children.push(
              tooltipRow(meta[candidates[position].index], texts[position])
            )
          }
          if (withMore || shown > count) {
            children.push(moreLine(Math.max(1, shown - count)))
          }
          tooltip?.replaceChildren(...children)
        }

        const { originX, originY } = geometry
        const limit = geometry.height - 2 * tooltipEdge
        render(candidates.length)
        tooltip.hidden = false
        if (tooltip.offsetHeight > limit && candidates.length > 1) {
          render(candidates.length, true)
          const rowElements =
            tooltip.querySelectorAll<HTMLElement>(".ts-tooltip-row")
          const step =
            rowElements.length > 1
              ? rowElements[1].offsetTop - rowElements[0].offsetTop
              : 0
          const fit =
            step > 0
              ? candidates.length -
                Math.ceil((tooltip.offsetHeight - limit) / step)
              : 1
          render(Math.max(1, Math.min(candidates.length - 1, fit)))
        }

        const width = tooltip.offsetWidth
        const height = tooltip.offsetHeight
        let x = originX + left + tooltipGap
        if (x + width > geometry.width - tooltipEdge) {
          x = originX + left - width - tooltipGap
        }
        const top = hovered ? (u.cursor.top ?? 0) : 0
        const y = originY + top - height / 2
        const maxX = Math.max(tooltipEdge, geometry.width - width - tooltipEdge)
        const maxY = Math.max(
          tooltipEdge,
          geometry.height - height - tooltipEdge
        )
        tooltip.style.transform = `translate(${Math.min(Math.max(tooltipEdge, x), maxX)}px, ${Math.min(Math.max(tooltipEdge, y), maxY)}px)`
      },
    },
  }
}

export function thresholdsPlugin(
  thresholds: Threshold[],
  mode: "absolute" | "percentage",
  resolveColor: (color: string) => string
): uPlot.Plugin {
  const colors = thresholds.map((threshold) => resolveColor(threshold.color))
  return {
    hooks: {
      draw: (u) => {
        const yScale = u.scales.y
        const { min, max } = yScale
        if (min == null || max == null) return
        const { ctx } = u
        const { left, top, width, height } = u.bbox
        const ratio = uPlot.pxRatio
        ctx.save()
        ctx.beginPath()
        ctx.rect(left, top, width, height)
        ctx.clip()
        ctx.lineWidth = Math.max(1, Math.round(ratio))
        ctx.setLineDash([4 * ratio, 4 * ratio])
        thresholds.forEach((threshold, index) => {
          if (threshold.value === undefined) return
          const at =
            mode === "percentage"
              ? min + (threshold.value / 100) * (max - min)
              : threshold.value
          if (at < min || at > max) return
          const y = Math.round(u.valToPos(at, "y", true))
          ctx.strokeStyle = colors[index]
          ctx.beginPath()
          ctx.moveTo(left, y)
          ctx.lineTo(left + width, y)
          ctx.stroke()
        })
        ctx.restore()
      },
    },
  }
}
