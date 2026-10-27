import type { DisplayOption } from "./types"
import { calculation, legend } from "./shared"

export const piechart: readonly DisplayOption[] = [
  calculation,
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
