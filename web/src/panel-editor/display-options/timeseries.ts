import type { DisplayOption } from "./types"
import {
  legend,
  legendHideEmpty,
  legendHideZero,
  legendMode,
  legendSortBy,
  legendSortDesc,
  legendValues,
  legendWidth,
  tooltip,
  xAxis,
} from "./shared"

export const timeseries: readonly DisplayOption[] = [
  legend,
  legendValues,
  legendMode,
  legendSortBy,
  legendSortDesc,
  legendWidth,
  legendHideZero,
  legendHideEmpty,
  tooltip,
  xAxis,
]
