import {
  CircleCheckIcon,
  CircleDotIcon,
  InfoIcon,
  OctagonAlertIcon,
  TriangleAlertIcon,
  type LucideIcon,
} from "lucide-react"

import { formatValue, ownDecimals } from "@/lib/units"
import type { Threshold, Unit } from "@/spec"

export type ThresholdLevel =
  "base" | "ok" | "info" | "warning" | "critical" | "custom"

export type ThresholdState = {
  level: ThresholdLevel
  color: string | null
  label: string
  threshold?: Threshold
}

const semanticLabels = {
  ok: "OK",
  info: "Info",
  warning: "Warning",
  critical: "Critical",
} as const

export const thresholdIcons: Record<
  Exclude<ThresholdLevel, "base">,
  LucideIcon
> = {
  ok: CircleCheckIcon,
  info: InfoIcon,
  warning: TriangleAlertIcon,
  critical: OctagonAlertIcon,
  custom: CircleDotIcon,
}

const baseState: ThresholdState = { level: "base", color: null, label: "" }

function isSemantic(color: string): color is keyof typeof semanticLabels {
  return Object.hasOwn(semanticLabels, color)
}

export function thresholdColor(color: string): string {
  return isSemantic(color) ? `var(--threshold-${color})` : color
}

function labelDecimals(
  value: number,
  unit?: Unit,
  decimals?: number
): number | undefined {
  const own = ownDecimals(value, unit)
  if (own === undefined) return decimals
  if (decimals === undefined) return own
  return Math.max(decimals, own)
}

function stateOf(
  threshold: Threshold,
  unit?: Unit,
  decimals?: number
): ThresholdState {
  if (isSemantic(threshold.color)) {
    return {
      level: threshold.color,
      color: thresholdColor(threshold.color),
      label: semanticLabels[threshold.color],
      threshold,
    }
  }
  const label =
    threshold.value === undefined
      ? "Base"
      : `At or above ${formatValue(threshold.value, unit, labelDecimals(threshold.value, unit, decimals))}`
  return { level: "custom", color: threshold.color, label, threshold }
}

export function thresholdFor(
  value: number | null,
  thresholds?: Threshold[],
  unit?: Unit,
  decimals?: number
): ThresholdState {
  if (value === null || !thresholds) return baseState
  const active = thresholds.findLast(
    (threshold) => value >= (threshold.value ?? -Infinity)
  )
  return active ? stateOf(active, unit, decimals) : baseState
}
