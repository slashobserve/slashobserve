import type { DisplayOption } from "./types"
import {
  calculation,
  orientation,
  allValues,
  reduceFields,
  rowLimit,
} from "./shared"

export const bargauge: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
  orientation,
]
