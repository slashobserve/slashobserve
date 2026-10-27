import type { DisplayOption } from "./types"

export const nodeGraph: readonly DisplayOption[] = [
  {
    kind: "choice",
    key: "layout",
    label: "Layout",
    options: [
      { value: "layered", label: "Layered" },
      { value: "grid", label: "Grid" },
    ],
  },
]
