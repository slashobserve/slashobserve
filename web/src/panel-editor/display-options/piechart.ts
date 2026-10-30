import type { DisplayOption } from "./types"
import {
  calculation,
  legend,
  allValues,
  reduceFields,
  rowLimit,
} from "./shared"

export const piechart: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
  {
    kind: "choice",
    key: "pieType",
    label: "Pie type",
    options: [
      { value: "pie", label: "Pie" },
      { value: "donut", label: "Donut" },
    ],
  },
  legend,
]
