import { useMemo } from "react"

import { commonDecimals, formatValue } from "@/lib/units"

import { firstNumberSeries, legendFor } from "../frames"
import { thresholdFor, thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"

const charWidth = 0.62

export const StatPanel: PanelRenderer = ({ panel, frames, width, height }) => {
  const tiles = useMemo(
    () => firstNumberSeries(frames, (frame) => legendFor(panel, frame)),
    [frames, panel]
  )

  const decimals = commonDecimals(
    tiles.map((tile) => tile.value),
    panel.unit,
    panel.decimals
  )

  return (
    <div
      className="grid gap-2 overflow-y-auto p-2 pt-0"
      style={{
        width,
        height,
        gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))",
        gridAutoRows: "minmax(72px, 1fr)",
      }}
    >
      {tiles.map((tile) => {
        const formatted = formatValue(tile.value, panel.unit, decimals)
        const state = thresholdFor(
          tile.value,
          panel.thresholds,
          panel.unit,
          decimals
        )
        const Icon = state.level === "base" ? null : thresholdIcons[state.level]
        const name = tiles.length > 1 && tile.label ? tile.label : panel.title
        return (
          <div
            key={tile.key}
            role="group"
            aria-label={`${name}: ${formatted}${state.level !== "base" ? `, ${state.label}` : ""}`}
            data-threshold={state.level}
            className="[container-type:size] flex min-h-0 flex-col items-center justify-center gap-1 rounded-md px-2"
            style={
              state.color
                ? {
                    backgroundColor: `color-mix(in oklch, ${state.color} 12%, transparent)`,
                  }
                : undefined
            }
          >
            <span
              className="leading-none font-semibold text-foreground tabular-nums"
              style={{
                fontSize: `clamp(1.25rem, min(${(94 / (Math.max(formatted.length, 3) * charWidth)).toFixed(1)}cqw, 45cqh), 3.5rem)`,
              }}
            >
              {formatted}
            </span>
            {Icon ? (
              <span className="flex items-center gap-1 text-xs font-medium text-foreground">
                <Icon
                  className="size-4"
                  aria-hidden="true"
                  style={{ color: state.color ?? undefined }}
                />
                {state.label}
              </span>
            ) : null}
            {tiles.length > 1 ? (
              <span
                title={tile.label}
                className="max-w-full truncate text-xs text-muted-foreground"
              >
                {tile.label}
              </span>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

export default StatPanel
