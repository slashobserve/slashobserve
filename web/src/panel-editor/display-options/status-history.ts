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

export const statusHistory: readonly DisplayOption[] = [
  showValues,
  rowHeight,
  {
    kind: "number",
    key: "columnWidth",
    label: "Column width",
    hint: "Share of the cell, from 0.1 to 1",
    rule: { min: 0.1, max: 1 },
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
