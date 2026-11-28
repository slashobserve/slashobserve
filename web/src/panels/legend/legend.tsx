import { useMemo, type Ref } from "react"

import { cn } from "@/lib/utils"
import type { LineStyle } from "../colors"
import type { DrawStyle } from "../timeseries/series-config"
import { useLegendTextMeasure } from "./legend-measure"
import { inlineValues, legendNames } from "./legend-model"
import { inlineValuesWidth, legendNameRoom } from "./legend-width"

export type LegendItem = {
  key: string
  label: string
  color: string
  line: LineStyle
  style: DrawStyle
}

export type LegendPlacement = "bottom" | "right"

export function LegendName({ label, text }: { label: string; text: string }) {
  return (
    <span title={label} className="min-w-0 truncate">
      {text}
    </span>
  )
}

function legendButtonName(label: string, values: string): string {
  return values ? `${label} ${values}` : label
}

export function useLegendNames(
  items: readonly LegendItem[],
  roomPx: number | null
): string[] {
  const measure = useLegendTextMeasure()
  return useMemo(() => {
    const measureName = measure(rootFontSize())
    return legendNames(
      items.map((item) => item.label),
      roomPx === null
        ? undefined
        : { px: roomPx, measure: (text) => measureName(text, 400) }
    )
  }, [items, roomPx, measure])
}

export const rowRem = 1.25
const gapRem = 0.125
export const padRem = 0.375

export function legendMaxHeight(available: number, rem: number): number {
  const rows = Math.max(
    1,
    Math.floor((available / rem - padRem + gapRem) / (rowRem + gapRem))
  )
  return (rows * rowRem + (rows - 1) * gapRem + padRem) * rem
}

export type LegendTableFit = {
  maxHeight: number
  scrolls: boolean
  footer: boolean
}

export function legendTableFit(
  available: number,
  rem: number,
  rowCount: number
): LegendTableFit {
  const slots = Math.max(
    1,
    Math.floor((available - (rowRem + padRem) * rem) / (rowRem * rem))
  )
  const scrolls = rowCount > slots
  return {
    maxHeight: (rowRem * (slots + 1) + padRem) * rem,
    scrolls,
    footer: scrolls && slots >= 2,
  }
}

export type LegendHover = (key: string | null) => void

export function hoverHandlers(key: string, onHover?: LegendHover) {
  if (!onHover) return {}
  return {
    onPointerEnter: () => onHover(key),
    onPointerLeave: () => onHover(null),
    onFocus: () => onHover(key),
    onBlur: () => onHover(null),
  }
}

export function rowButtonClass(visible: boolean): string {
  return cn(
    "flex max-w-full items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
    !visible && "opacity-50"
  )
}

export function LegendSwatch({
  color,
  line,
  style,
}: Pick<LegendItem, "color" | "line" | "style">) {
  return (
    <span
      data-slot="legend-swatch"
      data-line={line}
      data-shape={style}
      aria-hidden="true"
      className="series-swatch"
      style={{ color }}
    />
  )
}

let cachedRootFontSize: number | null = null

export function rootFontSize(): number {
  if (cachedRootFontSize === null) {
    cachedRootFontSize =
      parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
    window.addEventListener("resize", () => (cachedRootFontSize = null), {
      once: true,
    })
  }
  return cachedRootFontSize
}

type Props = {
  ref?: Ref<HTMLDivElement>
  items: LegendItem[]
  reducers?: readonly string[]
  headers?: readonly string[]
  cells?: string[][]
  hidden: ReadonlySet<string>
  placement: LegendPlacement
  available: number
  width: number
  onToggle: (key: string) => void
  onHover?: LegendHover | undefined
  more?: number
}

export function Legend({
  ref,
  items,
  reducers = [],
  headers,
  cells = [],
  hidden,
  placement,
  available,
  width,
  onToggle,
  onHover,
  more = 0,
}: Props) {
  const bottom = placement === "bottom"
  const measure = useLegendTextMeasure()
  const valuesPx =
    reducers.length > 0
      ? inlineValuesWidth(
          items.map((_, row) =>
            inlineValues(reducers, cells[row] ?? [], headers)
          ),
          measure(rootFontSize()),
          rootFontSize()
        )
      : 0
  const shownNames = useLegendNames(
    items,
    bottom ? null : legendNameRoom(width, rootFontSize(), valuesPx)
  )
  return (
    <div
      ref={ref}
      role="group"
      aria-label="Series"
      className={cn(
        "flex overflow-y-auto",
        bottom
          ? "shrink-0 flex-wrap gap-x-1 gap-y-0.5 px-2 pb-1.5"
          : "flex-col gap-y-0.5 px-1 pb-1.5"
      )}
      style={
        bottom
          ? { maxHeight: legendMaxHeight(available, rootFontSize()) }
          : { width }
      }
    >
      {items.map((item, row) => {
        const visible = !hidden.has(item.key)
        const values =
          reducers.length > 0
            ? inlineValues(reducers, cells[row] ?? [], headers)
            : ""
        return (
          <button
            key={item.key}
            type="button"
            aria-label={legendButtonName(item.label, values)}
            aria-pressed={visible}
            data-style={item.style}
            onClick={() => onToggle(item.key)}
            className={rowButtonClass(visible)}
            {...hoverHandlers(item.key, onHover)}
          >
            <LegendSwatch
              color={item.color}
              line={item.line}
              style={item.style}
            />
            <LegendName
              label={item.label}
              text={shownNames[row] ?? item.label}
            />
            {reducers.length > 0 ? (
              <span className="whitespace-pre text-muted-foreground">
                {values}
              </span>
            ) : null}
          </button>
        )
      })}
      {more > 0 ? (
        <span className="px-1.5 py-0.5 text-xs text-muted-foreground">
          +{more} more
        </span>
      ) : null}
    </div>
  )
}
