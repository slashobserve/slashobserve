import type { DisplayOption } from "./types"
import {
  calculation,
  orientation,
  allValues,
  reduceFields,
  rowLimit,
} from "./shared"

export const stat: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
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
  {
    kind: "choice",
    key: "justify",
    label: "Alignment",
    options: [
      { value: "auto", label: "Auto" },
      { value: "center", label: "Center" },
    ],
  },
  {
    kind: "number",
    key: "titleSize",
    label: "Title size",
    hint: "A whole number of pixels from 6 to 200. Empty is 13.",
    rule: { min: 6, max: 200, integer: true },
  },
  {
    kind: "number",
    key: "valueSize",
    label: "Value size",
    hint: "A whole number of pixels from 6 to 200. Empty is 28.",
    rule: { min: 6, max: 200, integer: true },
  },
  { kind: "toggle", key: "percentChange", label: "Show percent change" },
  {
    kind: "choice",
    key: "percentChangeColor",
    label: "Percent change color",
    options: [
      { value: "standard", label: "Standard" },
      { value: "inverted", label: "Inverted" },
      { value: "value", label: "Same as value" },
    ],
  },
  { kind: "toggle", key: "wideLayout", label: "Wide layout" },
]
