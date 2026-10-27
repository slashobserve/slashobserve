import type { DisplayOption } from "./types"

export const histogram: readonly DisplayOption[] = [
  {
    kind: "number",
    key: "bucketSize",
    label: "Bucket size",
    hint: "A number above 0. Empty is automatic.",
    rule: { min: 0, minExclusive: true, max: Infinity },
  },
  {
    kind: "number",
    key: "bucketCount",
    label: "Bucket count",
    hint: "A whole number from 1 to 1000. Empty is automatic.",
    rule: { min: 1, max: 1000, integer: true },
  },
]
