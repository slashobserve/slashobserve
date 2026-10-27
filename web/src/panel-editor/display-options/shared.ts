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

export const legendValues: DisplayOption = {
  kind: "reducers",
  key: "legendValues",
  label: "Legend values",
  options: [
    { value: "lastNotNull", label: "Last" },
    { value: "mean", label: "Mean" },
    { value: "max", label: "Max" },
    { value: "min", label: "Min" },
    { value: "sum", label: "Total" },
  ],
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

export const maxFieldNameLength = 200

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
