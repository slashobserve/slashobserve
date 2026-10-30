import uPlot from "uplot"

import type { Threshold } from "@/spec"

import { resolveColor, type LineStyle } from "../colors"
import type { FieldConfig } from "../field-config"
import type { CursorMember } from "../shared-cursor"
import { exemplarAt, type ExemplarPoint } from "./exemplars"
import {
  bandBase,
  nearestSpan,
  thresholdValueAt,
  tooltipAnchor,
} from "./plot-model"
import { formatTogether, type DrawStyle } from "./series-config"
import type { Cell } from "./stack"
import {
  candidateRows,
  tooltipRows,
  type StackEntry,
  type TooltipRow,
  type TooltipSort,
} from "./tooltip-rows"

const maxTooltipRows = 10
const tooltipGap = 12
const tooltipEdge = 4
const markerSize = 7
const hitRadius = 6

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
  hideInTooltip: boolean
  rowColor?: (row: number) => string
}

export type TooltipView = {
  sort: TooltipSort
  hideZeros: boolean
  cumulative: boolean
  maxWidth?: number
  maxHeight?: number
}

type Row = TooltipRow

function stackAt(entries: TooltipSeries[], idx: number): StackEntry[] {
  return entries.map(({ stacked, below, values }) => {
    const value = values[idx]
    return { stacked, below, value: typeof value === "number" ? value : 0 }
  })
}

function swatch(color: string, line: LineStyle, style: DrawStyle): HTMLElement {
  const element = document.createElement("span")
  element.className = "series-swatch"
  element.dataset.line = line
  element.dataset.shape = style
  element.style.color = color
  return element
}

function tooltipRow(
  entry: TooltipSeries,
  text: string,
  row: number
): HTMLElement {
  const line = document.createElement("div")
  line.className = "ts-tooltip-row"
  const label = document.createElement("span")
  label.className = "ts-tooltip-label"
  label.textContent = entry.label
  const value = document.createElement("span")
  value.className = "ts-tooltip-value"
  value.textContent = text
  line.append(
    swatch(entry.rowColor?.(row) ?? entry.color, entry.line, entry.style),
    label,
    value
  )
  return line
}

export type ExemplarLayer = {
  points: ExemplarPoint[]
  seriesKeys: string[]
}

type ExemplarHit = { point: ExemplarPoint; entry: TooltipSeries | undefined }

function shownExemplars(
  u: uPlot,
  first: number,
  layer: ExemplarLayer
): { point: ExemplarPoint; index: number }[] {
  const indexOf = new Map(layer.seriesKeys.map((key, index) => [key, index]))
  return layer.points.flatMap((point) => {
    if (point.seriesKey === undefined) return [{ point, index: -1 }]
    const index = indexOf.get(point.seriesKey)
    return index !== undefined && u.series[first + index]?.show
      ? [{ point, index }]
      : []
  })
}

function exemplarHit(
  u: uPlot,
  first: number,
  layer: ExemplarLayer,
  entries: TooltipSeries[],
  left: number,
  top: number
): ExemplarHit | null {
  const shown = shownExemplars(u, first, layer)
  const point = exemplarAt(
    shown.map((item) => item.point),
    left,
    top,
    hitRadius,
    (candidate) => [
      u.valToPos(candidate.x, "x"),
      u.valToPos(candidate.y, candidate.scale),
    ]
  )
  if (!point) return null
  const index = shown.find((item) => item.point === point)?.index ?? -1
  return { point, entry: index >= 0 ? entries[index] : undefined }
}

function exemplarSection(hit: ExemplarHit, config: FieldConfig): HTMLElement {
  const section = document.createElement("div")
  section.className = "mb-1 border-b border-border pb-1"
  const title = document.createElement("div")
  title.className = "font-medium"
  title.textContent = "Exemplar"
  const value = document.createElement("div")
  value.className = "tabular-nums"
  value.textContent = formatTogether([{ value: hit.point.y, config }])[0]
  const list = document.createElement("dl")
  list.className = "mt-0.5 flex flex-col text-muted-foreground"
  for (const [key, text] of hit.point.labels) {
    const row = document.createElement("div")
    row.className = "flex gap-1.5"
    const term = document.createElement("dt")
    term.textContent = `${key}:`
    const detail = document.createElement("dd")
    detail.className = "min-w-0 break-all text-popover-foreground"
    detail.textContent = text
    row.append(term, detail)
    list.append(row)
  }
  section.append(title, value, list)
  return section
}

export function exemplarsPlugin(
  first: number,
  layer: () => ExemplarLayer,
  series: () => TooltipSeries[]
): uPlot.Plugin {
  return {
    hooks: {
      draw: (u) => {
        const { points } = layer()
        if (points.length === 0) return
        const colors = series().map((entry) => resolveColor(entry.color))
        const fallback = resolveColor("var(--muted-foreground)")
        const outline = resolveColor("var(--background)")
        const { ctx } = u
        const { left, top, width, height } = u.bbox
        const ratio = uPlot.pxRatio
        const half = (markerSize * ratio) / 2
        ctx.save()
        ctx.beginPath()
        ctx.rect(left, top, width, height)
        ctx.clip()
        ctx.lineWidth = Math.max(1, Math.round(ratio))
        ctx.strokeStyle = outline
        for (const { point, index } of shownExemplars(u, first, layer())) {
          const x = left + u.valToPos(point.x, "x", true)
          const y = top + u.valToPos(point.y, point.scale, true)
          if (!Number.isFinite(x) || !Number.isFinite(y)) continue
          ctx.fillStyle = colors[index] ?? fallback
          ctx.beginPath()
          ctx.moveTo(x, y - half)
          ctx.lineTo(x + half, y)
          ctx.lineTo(x, y + half)
          ctx.lineTo(x - half, y)
          ctx.closePath()
          ctx.fill()
          ctx.stroke()
        }
        ctx.restore()
      },
    },
  }
}

function moreLine(count: number): HTMLElement {
  const more = document.createElement("div")
  more.className = "ts-tooltip-more"
  more.textContent = `${count} more`
  return more
}

function nearestRow(
  u: uPlot,
  rows: Row[],
  entries: TooltipSeries[],
  first: number,
  idx: number,
  top: number
): Row {
  const spans = rows.map((row): [number, number] => {
    const entry = entries[row.index]
    const drawn = u.data[first + row.index][idx] as number
    const lower =
      entry.below === null ? null : { drawn: u.data[first + entry.below][idx] }
    const base = bandBase(drawn, entry.stacked, lower)
    return [u.valToPos(drawn, entry.scale), u.valToPos(base, entry.scale)]
  })
  return rows[nearestSpan(spans, top)]
}

export function tooltipPlugin(
  host: HTMLElement,
  mode: "single" | "all",
  view: TooltipView,
  first: number,
  series: () => TooltipSeries[],
  heading: (x: number) => string,
  peerTooltip: () => boolean,
  exemplars: () => ExemplarLayer
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
        tooltip.style.overflow = "hidden"
        if (view.maxWidth) tooltip.style.maxWidth = `${view.maxWidth}px`
        if (view.maxHeight) tooltip.style.maxHeight = `${view.maxHeight}px`
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
        const anchor = tooltipAnchor(
          u.cursor,
          u.over.matches(":hover"),
          peerTooltip()
        )
        if (!tooltip || !anchor) return hide()
        const { idx, left, top: cursorTop } = anchor
        const rowsMode = anchor.peer ? "all" : mode

        const meta = series()
        const rows = candidateRows(
          meta,
          idx,
          (index) => u.series[first + index]?.show === true
        )
        if (rows.length === 0) return hide()
        const listed =
          rowsMode === "single"
            ? {
                rows: [nearestRow(u, rows, meta, first, idx, cursorTop)],
                total: 1,
              }
            : tooltipRows(
                rows,
                { ...view, stack: stackAt(meta, idx) },
                maxTooltipRows
              )
        if (listed.rows.length === 0) return hide()
        const candidates = listed.rows
        const shown = listed.total
        const texts = formatTogether(
          candidates.map((row) => ({
            value: row.value,
            config: meta[row.index].config,
          }))
        )

        const title = document.createElement("div")
        title.className = "ts-tooltip-time"
        title.textContent = heading(u.data[0][idx])

        const hit = anchor.peer
          ? null
          : exemplarHit(u, first, exemplars(), meta, left, cursorTop)
        const section = hit
          ? exemplarSection(
              hit,
              (
                hit.entry ??
                meta.find((entry) => entry.scale === "y") ??
                meta[0]
              ).config
            )
          : null

        const render = (count: number, withMore = false) => {
          const children: HTMLElement[] = section ? [title, section] : [title]
          for (let position = 0; position < count; position++) {
            children.push(
              tooltipRow(meta[candidates[position].index], texts[position], idx)
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
        const y = originY + cursorTop - height / 2
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

export function cursorPlugin(member: {
  current: CursorMember | null
}): uPlot.Plugin {
  const onLeave = () => member.current?.move(null)
  return {
    hooks: {
      init: (u) => u.over.addEventListener("mouseleave", onLeave),
      setCursor: (u) => {
        const { left } = u.cursor
        if (left == null || left < 0 || !u.over.matches(":hover")) return
        member.current?.move(u.posToVal(left, "x") * 1000)
      },
      destroy: (u) => u.over.removeEventListener("mouseleave", onLeave),
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
          const at = thresholdValueAt(threshold.value, mode, min, max)
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
