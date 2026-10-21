import type { Ref } from "react"

import type { Reducer } from "@/spec"

import { reducerLabels } from "../reducers"
import {
  LegendSwatch,
  legendMaxHeight,
  rootFontSize,
  rowButtonClass,
  rowRem,
  type LegendItem,
  type LegendPlacement,
} from "./legend"

type Props = {
  ref?: Ref<HTMLDivElement>
  items: LegendItem[]
  reducers: Reducer[]
  cells: string[][]
  hidden: ReadonlySet<string>
  placement: LegendPlacement
  available: number
  width: number
  onToggle: (key: string) => void
}

export function LegendTable({
  ref,
  items,
  reducers,
  cells,
  hidden,
  placement,
  available,
  width,
  onToggle,
}: Props) {
  const bottom = placement === "bottom"
  const rem = rootFontSize()
  const header = rowRem * rem
  return (
    <div
      ref={ref}
      role="group"
      aria-label="Series"
      className="shrink-0 overflow-auto px-1 pb-1.5"
      style={
        bottom
          ? { maxHeight: legendMaxHeight(available - header, rem) + header }
          : { width }
      }
    >
      <table className="w-full border-separate border-spacing-0 text-xs">
        <thead>
          <tr>
            <th scope="col" className="sticky top-0 bg-background">
              <span className="sr-only">Series</span>
            </th>
            {reducers.map((reducer) => (
              <th
                key={reducer}
                scope="col"
                className="sticky top-0 bg-background px-1.5 py-0.5 text-right font-medium whitespace-nowrap text-muted-foreground tabular-nums"
              >
                {reducerLabels[reducer]}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((item, row) => {
            const visible = !hidden.has(item.key)
            return (
              <tr key={item.key} data-style={item.style}>
                <th
                  scope="row"
                  className="w-full max-w-0 min-w-16 p-0 text-left font-normal"
                >
                  <button
                    type="button"
                    aria-pressed={visible}
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
                </th>
                {cells[row].map((text, column) => (
                  <td
                    key={reducers[column]}
                    className={
                      visible
                        ? "px-1.5 py-0.5 text-right whitespace-nowrap text-muted-foreground tabular-nums"
                        : "px-1.5 py-0.5 text-right whitespace-nowrap text-muted-foreground tabular-nums opacity-50"
                    }
                  >
                    {text}
                  </td>
                ))}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
