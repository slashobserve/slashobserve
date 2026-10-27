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
]
