import type { DisplayOption } from "./types"

export const nodeGraph: readonly DisplayOption[] = [
  {
    kind: "choice",
    key: "layout",
    label: "Layout",
    options: [
      { value: "layered", label: "Layered" },
      { value: "force", label: "Force" },
      { value: "grid", label: "Grid" },
    ],
  },
  { kind: "toggle", key: "wheelZoom", label: "Zoom with the scroll wheel" },
]
