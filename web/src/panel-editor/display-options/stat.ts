import type { DisplayOption } from "./types"
import { calculation, orientation } from "./shared"

export const stat: readonly DisplayOption[] = [
  calculation,
  {
    kind: "choice",
    key: "colorMode",
    label: "Color mode",
    options: [
      { value: "value", label: "Value" },
      { value: "background", label: "Background" },
      { value: "none", label: "None" },
    ],
  },
  { kind: "toggle", key: "graph", label: "Show graph" },
  {
    kind: "choice",
    key: "textMode",
    label: "Text",
    absent: "Auto",
    options: [
      { value: "value", label: "Value" },
      { value: "name", label: "Name" },
      { value: "valueAndName", label: "Value and name" },
      { value: "none", label: "None" },
    ],
  },
  orientation,
]
