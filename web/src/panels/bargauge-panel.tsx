import { useMemo } from "react"

import { commonDecimals, formatValue } from "@/lib/units"

import { firstNumberSeries, legendFor } from "./frames"
import { thresholdFor, thresholdIcons } from "./thresholds"
import type { PanelRenderer } from "./types"

export const BargaugePanel: PanelRenderer = ({
  panel,
  frames,
  width,
  height,
}) => {
  const rows = useMemo(
    () =>
      firstNumberSeries(frames, (frame) => legendFor(panel, frame)).sort(
        (a, b) => {
          if (a.value === null || b.value === null) {
            return a.value === b.value ? 0 : a.value === null ? 1 : -1
          }
          return b.value - a.value
        }
      ),
    [frames, panel]
  )

  const min = panel.min ?? 0
  let max = panel.max ?? Math.max(...rows.map((row) => row.value ?? min))
  if (max <= min) max = min + 1

  const decimals = commonDecimals(
    rows.map((row) => row.value),
    panel.unit,
    panel.decimals
  )
  const formatted = rows.map((row) =>
    formatValue(row.value, panel.unit, decimals)
  )
  const valueWidth = Math.max(...formatted.map((text) => text.length), 1)
  const states = rows.map((row) =>
    thresholdFor(row.value, panel.thresholds, panel.unit, decimals)
  )
  const stateWidth = Math.max(...states.map((state) => state.label.length), 0)
  const labelWidth = Math.max(...rows.map((row) => row.label.length), 1)
  const columns = [
    `minmax(0, min(${labelWidth}ch, 40%))`,
    "minmax(0, 1fr)",
    ...(stateWidth > 0 ? [`calc(${stateWidth}ch + 1.25rem)`] : []),
    `${valueWidth}ch`,
  ].join(" ")

  return (
    <div
      role="region"
      aria-label={`${panel.title} values`}
      tabIndex={0}
      className="flex flex-col gap-2 overflow-y-auto px-3 pt-1 pb-2 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
      style={{ width, height }}
    >
      {rows.map((row, index) => {
        const state = states[index]
        const Icon = state.level === "base" ? null : thresholdIcons[state.level]
        const fraction =
          row.value === null
            ? 0
            : Math.min(1, Math.max(0, (row.value - min) / (max - min)))
        const text = formatted[index]
        return (
          <div
            key={row.key}
            role="meter"
            aria-label={row.label}
            aria-valuenow={row.value ?? undefined}
            aria-valuemin={min}
            aria-valuemax={max}
            aria-valuetext={state.label ? `${text}, ${state.label}` : text}
            data-threshold={state.level}
            className="grid shrink-0 items-center gap-x-3 text-xs"
            style={{ gridTemplateColumns: columns }}
          >
            <span title={row.label} className="truncate">
              {row.label}
            </span>
            <span className="h-2 overflow-hidden rounded-full bg-muted">
              <span
                className="block h-full rounded-full"
                style={{
                  width: `${fraction * 100}%`,
                  backgroundColor: state.color ?? "var(--series-1)",
                }}
              />
            </span>
            {stateWidth > 0 ? (
              <span className="flex items-center gap-1 font-medium">
                {Icon ? (
                  <Icon
                    className="size-3.5"
                    aria-hidden="true"
                    style={{ color: state.color ?? undefined }}
                  />
                ) : null}
                {state.label}
              </span>
            ) : null}
            <span className="text-right tabular-nums">{text}</span>
          </div>
        )
      })}
    </div>
  )
}
