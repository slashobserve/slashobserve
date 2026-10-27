import type { DisplayOption } from "./types"
import { fieldOption, legend, tooltip } from "./shared"

export const xychart: readonly DisplayOption[] = [
  fieldOption("xField", "X field", "First number field"),
  {
    kind: "choice",
    key: "show",
    label: "Show",
    options: [
      { value: "points", label: "Points" },
      { value: "lines", label: "Lines" },
      { value: "points+lines", label: "Points and lines" },
    ],
  },
  {
    kind: "number",
    key: "pointSize",
    label: "Point size",
    hint: "A whole number from 1 to 40.",
    rule: { min: 1, max: 40, integer: true },
  },
  legend,
  tooltip,
]
