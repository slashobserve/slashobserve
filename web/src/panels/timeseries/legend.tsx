import type { Ref } from "react"

import { cn } from "@/lib/utils"

import type { SeriesStyle } from "../colors"

export type LegendItem = { key: string; label: string; style: SeriesStyle }

type Props = {
  ref?: Ref<HTMLDivElement>
  items: LegendItem[]
  hidden: ReadonlySet<string>
  maxHeight: number
  onToggle: (key: string) => void
}

export function Legend({ ref, items, hidden, maxHeight, onToggle }: Props) {
  return (
    <div
      ref={ref}
      role="group"
      aria-label="Series"
      className="flex shrink-0 flex-wrap gap-x-1 gap-y-0.5 overflow-y-auto px-2 pb-1.5"
      style={{ maxHeight }}
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
              "flex max-w-full items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50",
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
