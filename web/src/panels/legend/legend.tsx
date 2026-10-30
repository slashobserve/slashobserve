import type { Ref } from "react"

import { cn } from "@/lib/utils"

import type { LineStyle } from "../colors"
import type { DrawStyle } from "../timeseries/series-config"

export type LegendItem = {
  key: string
  label: string
  color: string
  line: LineStyle
  style: DrawStyle
}

export type LegendPlacement = "bottom" | "right"

export const rowRem = 1.25
const gapRem = 0.125
const padRem = 0.375

export function legendMaxHeight(available: number, rem: number): number {
  const rows = Math.max(
    1,
    Math.floor((available / rem - padRem + gapRem) / (rowRem + gapRem))
  )
  return (rows * rowRem + (rows - 1) * gapRem + padRem) * rem
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
  hidden: ReadonlySet<string>
  placement: LegendPlacement
  available: number
  width: number
  onToggle: (key: string) => void
}

export function Legend({
  ref,
  items,
  hidden,
  placement,
  available,
  width,
  onToggle,
}: Props) {
  const bottom = placement === "bottom"
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
      {items.map((item) => {
        const visible = !hidden.has(item.key)
        return (
          <button
            key={item.key}
            type="button"
            aria-pressed={visible}
            data-style={item.style}
            onClick={() => onToggle(item.key)}
            className={rowButtonClass(visible)}
          >
            <LegendSwatch
              color={item.color}
              line={item.line}
              style={item.style}
            />
            <span className="truncate">{item.label}</span>
          </button>
        )
      })}
    </div>
  )
}
