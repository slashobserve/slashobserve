import { fieldOption } from "./shared"
import type { DisplayOption } from "./types"

const maxDurationMs = 86400000

export const traces: readonly DisplayOption[] = [
  fieldOption("spanSearch", "Search", "Service, operation, id or attribute"),
  {
    kind: "names",
    key: "serviceNames",
    label: "Services",
    max: 50,
    placeholder: "All services",
  },
  { kind: "toggle", key: "excludeServices", label: "Exclude these services" },
  {
    kind: "names",
    key: "spanNames",
    label: "Operations",
    max: 50,
    placeholder: "All operations",
  },
  {
    kind: "toggle",
    key: "excludeSpanNames",
    label: "Exclude these operations",
  },
  {
    kind: "number",
    key: "minSpanDuration",
    label: "Shortest span",
    hint: "Milliseconds, from 0 to 86,400,000.",
    rule: { min: 0, max: maxDurationMs },
  },
  {
    kind: "number",
    key: "maxSpanDuration",
    label: "Longest span",
    hint: "Milliseconds, from 0 to 86,400,000.",
    rule: { min: 0, max: maxDurationMs },
  },
  { kind: "toggle", key: "matchesOnly", label: "Show matching spans only" },
  {
    kind: "toggle",
    key: "criticalPathOnly",
    label: "Show the critical path only",
  },
]
