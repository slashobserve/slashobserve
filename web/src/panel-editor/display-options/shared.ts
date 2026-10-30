import type { DisplayOption } from "./types"

export const reducerOptions = [
  { value: "lastNotNull", label: "Last" },
  { value: "last", label: "Last, including empty" },
  { value: "first", label: "First" },
  { value: "firstNotNull", label: "First, not empty" },
  { value: "min", label: "Min" },
  { value: "max", label: "Max" },
  { value: "mean", label: "Mean" },
  { value: "median", label: "Median" },
  { value: "sum", label: "Total" },
  { value: "count", label: "Count" },
  { value: "range", label: "Range" },
  { value: "delta", label: "Delta" },
  { value: "diff", label: "Difference" },
  { value: "stdDev", label: "Standard deviation" },
  { value: "distinctCount", label: "Distinct values" },
  { value: "changeCount", label: "Changes" },
]

export const calculation: DisplayOption = {
  kind: "reducer",
  key: "reduce",
  label: "Calculation",
}

export const allValues: DisplayOption = {
  kind: "toggle",
  key: "allValues",
  label: "Show every value",
}

export const rowLimit: DisplayOption = {
  kind: "number",
  key: "limit",
  label: "Row limit",
  hint: "Rows shown per field when every value is shown",
  rule: { min: 1, max: 5000, integer: true },
}

export const legend: DisplayOption = {
  kind: "choice",
  key: "legend",
  label: "Legend",
  options: [
    { value: "bottom", label: "Bottom" },
    { value: "right", label: "Right" },
    { value: "hidden", label: "Hidden" },
  ],
}

const legendReducerOptions = [
  { value: "lastNotNull", label: "Last" },
  { value: "mean", label: "Mean" },
  { value: "max", label: "Max" },
  { value: "min", label: "Min" },
  { value: "sum", label: "Total" },
]

export const legendValues: DisplayOption = {
  kind: "reducers",
  key: "legendValues",
  label: "Legend values",
  options: legendReducerOptions,
}

export const legendMode: DisplayOption = {
  kind: "choice",
  key: "legendMode",
  label: "Legend mode",
  absent: "Table when values are shown",
  options: [
    { value: "list", label: "List" },
    { value: "table", label: "Table" },
  ],
}

export const legendSortBy: DisplayOption = {
  kind: "choice",
  key: "legendSortBy",
  label: "Sort legend by",
  absent: "Series order",
  options: [{ value: "name", label: "Name" }, ...legendReducerOptions],
}

export const legendSortDesc: DisplayOption = {
  kind: "toggle",
  key: "legendSortDesc",
  label: "Sort descending",
}

export const legendWidth: DisplayOption = {
  kind: "number",
  key: "legendWidth",
  label: "Legend width",
  hint: "Pixels, for a legend on the right",
  rule: { min: 40, max: 2000, integer: true },
}

export const legendHideZero: DisplayOption = {
  kind: "toggle",
  key: "legendHideZero",
  label: "Hide series that are all zero",
}

export const legendHideEmpty: DisplayOption = {
  kind: "toggle",
  key: "legendHideEmpty",
  label: "Hide series with no values",
}

export const tooltip: DisplayOption = {
  kind: "choice",
  key: "tooltip",
  label: "Tooltip",
  options: [
    { value: "single", label: "Single series" },
    { value: "all", label: "All series" },
    { value: "hidden", label: "Hidden" },
  ],
}

export const tooltipSort: DisplayOption = {
  kind: "choice",
  key: "tooltipSort",
  label: "Tooltip sort",
  options: [
    { value: "none", label: "Series order" },
    { value: "asc", label: "Ascending" },
    { value: "desc", label: "Descending" },
  ],
}

export const tooltipHideZeros: DisplayOption = {
  kind: "toggle",
  key: "tooltipHideZeros",
  label: "Hide zero values in the tooltip",
}

export const tooltipCumulative: DisplayOption = {
  kind: "toggle",
  key: "tooltipCumulative",
  label: "Show stacked totals in the tooltip",
}

export const tooltipMaxWidth: DisplayOption = {
  kind: "number",
  key: "tooltipMaxWidth",
  label: "Tooltip max width",
  hint: "Pixels",
  rule: { min: 100, max: 2000, integer: true },
}

export const tooltipMaxHeight: DisplayOption = {
  kind: "number",
  key: "tooltipMaxHeight",
  label: "Tooltip max height",
  hint: "Pixels",
  rule: { min: 100, max: 2000, integer: true },
}

export const xAxis: DisplayOption = {
  kind: "choice",
  key: "xAxis",
  label: "X axis",
  options: [
    { value: "shown", label: "Shown" },
    { value: "hidden", label: "Hidden" },
  ],
}

export const orientation: DisplayOption = {
  kind: "choice",
  key: "orientation",
  label: "Orientation",
  options: [
    { value: "auto", label: "Auto" },
    { value: "horizontal", label: "Horizontal" },
    { value: "vertical", label: "Vertical" },
  ],
}

export const showValues: DisplayOption = {
  kind: "choice",
  key: "showValues",
  label: "Show values",
  options: [
    { value: "auto", label: "Auto" },
    { value: "always", label: "Always" },
    { value: "never", label: "Never" },
  ],
}

export const rowHeight: DisplayOption = {
  kind: "number",
  key: "rowHeight",
  label: "Row height",
  hint: "Share of the row, from 0.1 to 1",
  rule: { min: 0.1, max: 1 },
}

const maxFieldNameLength = 200

export function fieldOption(
  key: string,
  label: string,
  placeholder: string
): DisplayOption {
  return {
    kind: "text",
    key,
    label,
    placeholder,
    maxLength: maxFieldNameLength,
  }
}

export const reduceFields = fieldOption(
  "fields",
  "Fields",
  "A field name or /regex/"
)
