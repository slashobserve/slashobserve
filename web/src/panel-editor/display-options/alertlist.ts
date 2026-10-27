import type { DisplayOption } from "./types"

export const alertlist: readonly DisplayOption[] = [
  {
    kind: "number",
    key: "limit",
    label: "Limit",
    hint: "A whole number from 1 to 100.",
    rule: { min: 1, max: 100, integer: true },
  },
  {
    kind: "choice",
    key: "sort",
    label: "Sort",
    options: [
      { value: "alphaAsc", label: "A to Z" },
      { value: "alphaDesc", label: "Z to A" },
      { value: "importance", label: "Importance" },
      { value: "timeAsc", label: "Oldest first" },
      { value: "timeDesc", label: "Newest first" },
    ],
  },
  {
    kind: "choice",
    key: "viewMode",
    label: "View",
    options: [
      { value: "list", label: "List" },
      { value: "stat", label: "Count" },
    ],
  },
]
