import type { DisplayOption } from "./types"

export const dashlist: readonly DisplayOption[] = [
  {
    kind: "text",
    key: "query",
    label: "Search",
    placeholder: "All dashboards",
    maxLength: 200,
  },
  { kind: "tags", key: "tags", label: "Tags", max: 10 },
  {
    kind: "number",
    key: "limit",
    label: "Limit",
    hint: "A whole number from 1 to 100.",
    rule: { min: 1, max: 100, integer: true },
  },
  { kind: "toggle", key: "keepTime", label: "Keep the time range" },
  { kind: "toggle", key: "includeVars", label: "Include variable values" },
]
