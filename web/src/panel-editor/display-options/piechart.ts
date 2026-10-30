import type { DisplayOption } from "./types"
import {
  calculation,
  legend,
  allValues,
  reduceFields,
  rowLimit,
  tooltip,
  tooltipHideZeros,
  tooltipMaxHeight,
  tooltipMaxWidth,
  tooltipSort,
  legendSortDesc,
  legendWidth,
} from "./shared"

export const piechart: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
  {
    kind: "choice",
    key: "pieType",
    label: "Pie type",
    options: [
      { value: "pie", label: "Pie" },
      { value: "donut", label: "Donut" },
    ],
  },
  {
    kind: "reducers",
    key: "sliceLabels",
    label: "Slice labels",
    options: [
      { value: "name", label: "Name" },
      { value: "value", label: "Value" },
      { value: "percent", label: "Percent" },
    ],
  },
  {
    kind: "number",
    key: "titleSize",
    label: "Label name size",
    hint: "A whole number of pixels from 1 to 200. Empty is 12.",
    rule: { min: 1, max: 200, integer: true },
  },
  {
    kind: "number",
    key: "valueSize",
    label: "Label value size",
    hint: "A whole number of pixels from 1 to 200. Empty is 12.",
    rule: { min: 1, max: 200, integer: true },
  },
  legend,
  {
    kind: "reducers",
    key: "legendShow",
    label: "Legend values",
    options: [
      { value: "value", label: "Value" },
      { value: "percent", label: "Percent" },
    ],
  },
  {
    kind: "choice",
    key: "legendMode",
    label: "Legend mode",
    options: [
      { value: "list", label: "List" },
      { value: "table", label: "Table" },
    ],
  },
  {
    kind: "choice",
    key: "legendSortBy",
    label: "Sort legend by",
    absent: "Series order",
    options: [
      { value: "name", label: "Name" },
      { value: "value", label: "Value" },
    ],
  },
  legendSortDesc,
  legendWidth,
  tooltip,
  tooltipSort,
  tooltipHideZeros,
  tooltipMaxWidth,
  tooltipMaxHeight,
]
