import type { Ref } from "react"

import { cn } from "@/lib/utils"

import type { SeriesStyle } from "../colors"

export type LegendItem = { key: string; label: string; style: SeriesStyle }

const rowRem = 1.25
const gapRem = 0.125
const padRem = 0.375

export function legendMaxHeight(available: number, rem: number): number {
  const rows = Math.max(
    1,
    Math.floor((available / rem - padRem + gapRem) / (rowRem + gapRem))
  )
  return (rows * rowRem + (rows - 1) * gapRem + padRem) * rem
}

let cachedRootFontSize: number | null = null

function rootFontSize(): number {
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
  available: number
  onToggle: (key: string) => void
}

export function Legend({ ref, items, hidden, available, onToggle }: Props) {
  return (
    <div
      ref={ref}
      role="group"
      aria-label="Series"
      className="flex shrink-0 flex-wrap gap-x-1 gap-y-0.5 overflow-y-auto px-2 pb-1.5"
      style={{ maxHeight: legendMaxHeight(available, rootFontSize()) }}
    >
      {items.map((item) => {
        const visible = !hidden.has(item.key)
        return (
          <button
            key={item.key}
            type="button"
            aria-pressed={visible}
            onClick={() => onToggle(item.key)}
            className={cn(
              "flex max-w-full items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
              !visible && "opacity-50"
            )}
          >
            <span
              data-slot="legend-swatch"
              data-line={item.style.line}
              aria-hidden="true"
              className="series-swatch"
              style={{ color: item.style.color }}
            />
            <span className="truncate">{item.label}</span>
          </button>
        )
      })}
    </div>
  )
}
