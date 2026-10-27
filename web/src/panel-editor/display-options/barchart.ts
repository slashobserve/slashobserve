import type { DisplayOption, NumberRule } from "./types"
import { fieldOption, legend, orientation, showValues, tooltip } from "./shared"

const share: NumberRule = { min: 0, minExclusive: true, max: 1 }
const shareHint = "A number above 0, at most 1."

export const barchart: readonly DisplayOption[] = [
  orientation,
  fieldOption("xField", "X field", "First text or time field"),
  showValues,
  {
    kind: "number",
    key: "barWidth",
    label: "Bar width",
    hint: shareHint,
    rule: share,
  },
  {
    kind: "number",
    key: "groupWidth",
    label: "Group width",
    hint: shareHint,
    rule: share,
  },
  legend,
  tooltip,
]
