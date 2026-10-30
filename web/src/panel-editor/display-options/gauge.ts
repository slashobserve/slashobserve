import type { DisplayOption } from "./types"
import { calculation, allValues, reduceFields, rowLimit } from "./shared"

export const gauge: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
  { kind: "toggle", key: "thresholdMarkers", label: "Show threshold markers" },
]
