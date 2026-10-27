import type { DisplayOption } from "./types"
import { fieldOption, legend, legendValues, tooltip } from "./shared"

export const trend: readonly DisplayOption[] = [
  fieldOption("xField", "X field", "First number field"),
  legend,
  legendValues,
  tooltip,
]
