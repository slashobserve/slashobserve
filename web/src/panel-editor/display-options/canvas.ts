import type { DisplayOption } from "./types"
import { calculation, semanticColorOptions } from "./shared"

export const canvas: readonly DisplayOption[] = [
  calculation,
  {
    kind: "choice",
    key: "backgroundColor",
    label: "Background color",
    absent: "None",
    options: semanticColorOptions,
  },
  {
    kind: "choice",
    key: "borderColor",
    label: "Border color",
    absent: "Default border",
    options: semanticColorOptions,
  },
  {
    kind: "number",
    key: "borderWidth",
    label: "Border width",
    hint: "Pixels, a whole number from 0 to 20",
    rule: { min: 0, max: 20, integer: true },
  },
  {
    kind: "number",
    key: "borderRadius",
    label: "Border radius",
    hint: "Pixels, a whole number from 0 to 100",
    rule: { min: 0, max: 100, integer: true },
  },
  { kind: "toggle", key: "panZoom", label: "Pan and zoom" },
  { kind: "toggle", key: "infinitePan", label: "Pan beyond the canvas" },
  {
    kind: "choice",
    key: "tooltip",
    label: "Tooltip",
    options: [
      { value: "details", label: "Details" },
      { value: "none", label: "None" },
    ],
  },
  {
    kind: "note",
    text: "Edit elements, connections and background images in the JSON view.",
  },
]
