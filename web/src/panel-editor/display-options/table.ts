import { fieldOption } from "./shared"
import type { DisplayOption } from "./types"

export const table: readonly DisplayOption[] = [
  { kind: "toggle", key: "header", label: "Show header" },
  {
    kind: "choice",
    key: "cellHeight",
    label: "Row height",
    options: [
      { value: "sm", label: "Small" },
      { value: "md", label: "Medium" },
      { value: "lg", label: "Large" },
    ],
  },
  { kind: "toggle", key: "typeIcons", label: "Show column type icons" },
  { kind: "toggle", key: "pagination", label: "Paginate rows" },
  {
    kind: "number",
    key: "frozenColumns",
    label: "Frozen columns",
    hint: "A whole number from 0 to 10.",
    rule: { min: 0, max: 10, integer: true },
  },
  fieldOption("sortBy", "Sort by", "A column name"),
  { kind: "toggle", key: "sortDesc", label: "Sort descending" },
  {
    kind: "reducers",
    key: "footer",
    label: "Footer",
    max: 5,
    options: [
      { value: "sum", label: "Total" },
      { value: "mean", label: "Mean" },
      { value: "min", label: "Min" },
      { value: "max", label: "Max" },
      { value: "lastNotNull", label: "Last" },
      { value: "count", label: "Count" },
    ],
  },
  { kind: "names", key: "footerFields", label: "Footer columns", max: 50 },
  { kind: "toggle", key: "footerCountRows", label: "Footer row count" },
]
