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
