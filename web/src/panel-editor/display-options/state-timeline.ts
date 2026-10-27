import type { DisplayOption } from "./types"
import { showValues } from "./shared"

export const stateTimeline: readonly DisplayOption[] = [
  { kind: "toggle", key: "mergeValues", label: "Merge equal values" },
  showValues,
]
