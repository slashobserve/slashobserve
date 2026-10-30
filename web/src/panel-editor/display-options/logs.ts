import type { DisplayOption } from "./types"

export const logs: readonly DisplayOption[] = [
  { kind: "toggle", key: "wrap", label: "Wrap lines" },
  { kind: "toggle", key: "showTime", label: "Show time" },
  {
    kind: "choice",
    key: "order",
    label: "Order",
    options: [
      { value: "newest", label: "Newest first" },
      { value: "oldest", label: "Oldest first" },
    ],
  },
  { kind: "toggle", key: "showLabels", label: "Show unique labels" },
  { kind: "toggle", key: "showCommonLabels", label: "Show common labels" },
  { kind: "toggle", key: "details", label: "Log details" },
  {
    kind: "choice",
    key: "detailsMode",
    label: "Details placement",
    absent: "Under the line",
    options: [{ value: "sidebar", label: "Side panel" }],
  },
  { kind: "toggle", key: "prettify", label: "Prettify JSON messages" },
  {
    kind: "choice",
    key: "dedup",
    label: "Deduplication",
    options: [
      { value: "none", label: "None" },
      { value: "exact", label: "Exact" },
      { value: "numbers", label: "Ignore numbers" },
      { value: "signature", label: "Ignore letters and numbers" },
    ],
  },
  { kind: "toggle", key: "controls", label: "Controls bar" },
  {
    kind: "choice",
    key: "fontSize",
    label: "Font size",
    options: [
      { value: "default", label: "Default" },
      { value: "small", label: "Small" },
    ],
  },
  { kind: "toggle", key: "highlight", label: "Highlight key=value and levels" },
  {
    kind: "toggle",
    key: "infiniteScroll",
    label: "Infinite scrolling (not applied yet)",
  },
]
