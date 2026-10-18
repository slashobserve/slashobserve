import uPlot from "uplot"

import { formatDateTime } from "@/lib/time"
import type { Threshold } from "@/spec"

import type { LineStyle } from "../colors"

const maxTooltipRows = 10
const tooltipGap = 12
const tooltipEdge = 4

export type TooltipSeries = { label: string; color: string; line: LineStyle }

type Row = { index: number; value: number }

function swatch(color: string, line: LineStyle): HTMLElement {
  const element = document.createElement("span")
  element.className = "series-swatch"
  element.dataset.line = line
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
  line.append(swatch(entry.color, entry.line), label, value)
  return line
}

function moreLine(count: number): HTMLElement {
  const more = document.createElement("div")
  more.className = "ts-tooltip-more"
  more.textContent = `${count} more`
  return more
}

export function tooltipPlugin(
  host: HTMLElement,
  series: () => TooltipSeries[],
  format: (values: number[]) => string[]
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
        for (let i = 1; i < u.series.length; i++) {
          const value = u.data[i]?.[idx]
          if (u.series[i].show && typeof value === "number") {
            rows.push({ index: i - 1, value })
          }
        }
        if (rows.length === 0) return hide()
        rows.sort((a, b) => b.value - a.value)
        const candidates = rows.slice(0, maxTooltipRows)
        const texts = format(candidates.map((row) => row.value))

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
          if (withMore || rows.length > count) {
            children.push(moreLine(Math.max(1, rows.length - count)))
          }
          tooltip?.replaceChildren(...children)
        }

        const { originX, originY } = geometry
        const limit = geometry.height - 2 * tooltipEdge
        render(candidates.length)
        tooltip.hidden = false
        if (tooltip.offsetHeight > limit && candidates.length > 1) {
          render(candidates.length, true)
          const first = tooltip.querySelectorAll<HTMLElement>(".ts-tooltip-row")
          const step =
            first.length > 1 ? first[1].offsetTop - first[0].offsetTop : 0
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
        const top = u.over.matches(":hover") ? (u.cursor.top ?? 0) : 0
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
          const at = threshold.value
          if (at === undefined || at < min || at > max) return
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
