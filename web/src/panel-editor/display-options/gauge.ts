import type { DisplayOption } from "./types"
import { calculation } from "./shared"

export const gauge: readonly DisplayOption[] = [
  calculation,
  { kind: "toggle", key: "thresholdMarkers", label: "Show threshold markers" },
]
