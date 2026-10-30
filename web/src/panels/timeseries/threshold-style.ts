import type { Threshold } from "@/spec"

import type { FieldConfig } from "../field-config"
import { thresholdValueAt } from "./plot-model"

export type ThresholdsStyle = FieldConfig["thresholdsStyle"]

export type ThresholdBand = { from: number; to: number; threshold: Threshold }

export const areaOpacity = 0.12

const parts: Record<
  ThresholdsStyle,
  { lines: "solid" | "dashed" | null; area: boolean; series: boolean }
> = {
  off: { lines: null, area: false, series: false },
  line: { lines: "solid", area: false, series: false },
  dashed: { lines: "dashed", area: false, series: false },
  area: { lines: null, area: true, series: false },
  "line+area": { lines: "solid", area: true, series: false },
  "dashed+area": { lines: "dashed", area: true, series: false },
  series: { lines: null, area: false, series: true },
}

export function thresholdParts(style: ThresholdsStyle) {
  return parts[style]
}

export function thresholdBands(
  list: readonly Threshold[],
  mode: FieldConfig["thresholdMode"],
  min: number,
  max: number
): ThresholdBand[] {
  const steps = list
    .map((threshold) => ({
      threshold,
      at:
        threshold.value === undefined
          ? -Infinity
          : thresholdValueAt(threshold.value, mode, min, max),
    }))
    .sort((a, b) => a.at - b.at)
  return steps.map(({ threshold, at }, index) => ({
    from: at,
    to: steps[index + 1]?.at ?? Infinity,
    threshold,
  }))
}
