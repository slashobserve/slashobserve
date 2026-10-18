import type uPlot from "uplot"

import { formatDateTime } from "@/lib/time"
import type { Threshold } from "@/spec"

import type { LineStyle } from "../colors"

const maxTooltipRows = 10
const tooltipGap = 12

export type TooltipSeries = { label: string; color: string; line: LineStyle }

type Row = { index: number; value: number }

function swatch(color: string, line: LineStyle): HTMLElement {
  const element = document.createElement("span")
  element.className = "series-swatch"
  element.dataset.line = line
  element.style.color = color
  return element
}

export function tooltipPlugin(
  series: () => TooltipSeries[],
  format: (values: number[]) => string[]
): uPlot.Plugin {
  let tooltip: HTMLDivElement | null = null

  function hide() {
    if (tooltip) tooltip.hidden = true
  }

  return {
    hooks: {
      init: (u) => {
        tooltip = document.createElement("div")
        tooltip.className = "ts-tooltip"
        tooltip.dataset.slot = "chart-tooltip"
        tooltip.setAttribute("aria-hidden", "true")
        tooltip.hidden = true
        u.over.appendChild(tooltip)
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
        const shown = rows.slice(0, maxTooltipRows)
        const texts = format(shown.map((row) => row.value))

        const heading = document.createElement("div")
        heading.className = "ts-tooltip-time"
        heading.textContent = formatDateTime(u.data[0][idx] * 1000)
        const children: HTMLElement[] = [heading]
        shown.forEach((row, position) => {
          const entry = meta[row.index]
          const line = document.createElement("div")
          line.className = "ts-tooltip-row"
          const label = document.createElement("span")
          label.className = "ts-tooltip-label"
          label.textContent = entry.label
          const value = document.createElement("span")
          value.className = "ts-tooltip-value"
          value.textContent = texts[position]
          line.append(swatch(entry.color, entry.line), label, value)
          children.push(line)
        })
        if (rows.length > shown.length) {
          const more = document.createElement("div")
          more.className = "ts-tooltip-more"
          more.textContent = `${rows.length - shown.length} more`
          children.push(more)
        }
        tooltip.replaceChildren(...children)
        tooltip.hidden = false

        const overWidth = u.over.clientWidth
        const overHeight = u.over.clientHeight
        const width = tooltip.offsetWidth
        const height = tooltip.offsetHeight
        let x = left + tooltipGap
        if (x + width > overWidth) x = left - width - tooltipGap
        const top = u.over.matches(":hover") ? (u.cursor.top ?? 0) : 0
        const y = Math.min(Math.max(0, top - height / 2), overHeight - height)
        tooltip.style.transform = `translate(${Math.max(0, x)}px, ${Math.max(0, y)}px)`
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
        const ratio = window.devicePixelRatio
        ctx.save()
        ctx.beginPath()
        ctx.rect(left, top, width, height)
        ctx.clip()
        ctx.lineWidth = Math.max(1, Math.round(ratio))
        ctx.setLineDash([4 * ratio, 4 * ratio])
        thresholds.forEach((threshold, index) => {
          if (threshold.value < min || threshold.value > max) return
          const y = Math.round(u.valToPos(threshold.value, "y", true))
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
