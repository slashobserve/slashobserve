import type { DisplayOption } from "./types"
import {
  fieldOption,
  legend,
  legendHideEmpty,
  legendHideZero,
  legendMode,
  legendSortBy,
  legendSortDesc,
  legendValues,
  legendWidth,
  tooltip,
} from "./shared"

export const trend: readonly DisplayOption[] = [
  fieldOption("xField", "X field", "First number field"),
  legend,
  legendValues,
  legendMode,
  legendSortBy,
  legendSortDesc,
  legendWidth,
  legendHideZero,
  legendHideEmpty,
  tooltip,
]
