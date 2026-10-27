import type { DisplayOption } from "./types"

export const table: readonly DisplayOption[] = [
  { kind: "toggle", key: "header", label: "Show header" },
  {
    kind: "reducers",
    key: "footer",
    label: "Footer",
    max: 5,
    options: [
      { value: "sum", label: "Total" },
      { value: "mean", label: "Mean" },
      { value: "min", label: "Min" },
      { value: "max", label: "Max" },
      { value: "lastNotNull", label: "Last" },
      { value: "count", label: "Count" },
    ],
  },
]
