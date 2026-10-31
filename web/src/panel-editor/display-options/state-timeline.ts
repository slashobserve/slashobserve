import type { DisplayOption } from "./types"
import {
  legend,
  legendMode,
  legendWidth,
  rowHeight,
  showValues,
  tooltip,
  tooltipHideZeros,
  tooltipMaxHeight,
  tooltipMaxWidth,
  tooltipSort,
} from "./shared"

export const stateTimeline: readonly DisplayOption[] = [
  { kind: "toggle", key: "mergeValues", label: "Merge equal values" },
  showValues,
  rowHeight,
  {
    kind: "number",
    key: "perPage",
    label: "Rows per page",
    hint: "A whole number from 1 to 100. Empty shows every row.",
    rule: { min: 1, max: 100, integer: true },
  },
  {
    kind: "choice",
    key: "alignValue",
    label: "Align values",
    options: [
      { value: "left", label: "Left" },
      { value: "center", label: "Center" },
      { value: "right", label: "Right" },
    ],
  },
  legend,
  legendMode,
  legendWidth,
  tooltip,
  tooltipSort,
  tooltipHideZeros,
  tooltipMaxWidth,
  tooltipMaxHeight,
]
