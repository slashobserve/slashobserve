import type { DisplayOption } from "./types"

export const heatmap: readonly DisplayOption[] = [
  { kind: "toggle", key: "calculate", label: "Calculate buckets" },
  {
    kind: "choice",
    key: "yScale",
    label: "Y scale",
    options: [
      { value: "linear", label: "Linear" },
      { value: "log", label: "Log" },
    ],
  },
]
