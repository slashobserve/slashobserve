import type { DisplayOption } from "./types"
import { calculation, allValues, reduceFields, rowLimit } from "./shared"

export const gauge: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
  { kind: "toggle", key: "thresholdMarkers", label: "Show threshold markers" },
  { kind: "toggle", key: "thresholdLabels", label: "Show threshold labels" },
  {
    kind: "number",
    key: "minWidth",
    label: "Minimum gauge width",
    hint: "Pixels, from 1 to 2000; the panel scrolls when gauges do not fit",
    rule: { min: 1, max: 2000, integer: true },
  },
  {
    kind: "number",
    key: "minHeight",
    label: "Minimum gauge height",
    hint: "Pixels, from 1 to 2000; the panel scrolls when gauges do not fit",
    rule: { min: 1, max: 2000, integer: true },
  },
  {
    kind: "number",
    key: "titleSize",
    label: "Title size",
    hint: "Pixels, from 1 to 200",
    rule: { min: 1, max: 200, integer: true },
  },
  {
    kind: "number",
    key: "valueSize",
    label: "Value size",
    hint: "Pixels, from 1 to 200",
    rule: { min: 1, max: 200, integer: true },
  },
]
