import type { DisplayOption } from "./types"
import {
  calculation,
  orientation,
  allValues,
  reduceFields,
  rowLimit,
} from "./shared"

export const bargauge: readonly DisplayOption[] = [
  calculation,
  allValues,
  rowLimit,
  reduceFields,
  orientation,
  {
    kind: "choice",
    key: "barStyle",
    label: "Bar style",
    options: [
      { value: "basic", label: "Basic" },
      { value: "lcd", label: "LCD" },
    ],
  },
  {
    kind: "choice",
    key: "valueMode",
    label: "Value",
    options: [
      { value: "color", label: "With threshold state" },
      { value: "text", label: "Plain text" },
      { value: "hidden", label: "Hidden" },
    ],
  },
  {
    kind: "choice",
    key: "namePlacement",
    label: "Name placement",
    options: [
      { value: "auto", label: "Auto" },
      { value: "top", label: "Top" },
      { value: "left", label: "Left" },
      { value: "hidden", label: "Hidden" },
    ],
  },
  { kind: "toggle", key: "showUnfilled", label: "Show unfilled area" },
  {
    kind: "number",
    key: "minWidth",
    label: "Minimum bar width",
    hint: "Pixels, from 1 to 2000, for vertical bars",
    rule: { min: 1, max: 2000, integer: true },
  },
  {
    kind: "number",
    key: "minHeight",
    label: "Minimum bar height",
    hint: "Pixels, from 1 to 2000; the panel scrolls when bars do not fit",
    rule: { min: 1, max: 2000, integer: true },
  },
  {
    kind: "number",
    key: "maxHeight",
    label: "Maximum bar height",
    hint: "Pixels, from 1 to 2000",
    rule: { min: 1, max: 2000, integer: true },
  },
  {
    kind: "number",
    key: "titleSize",
    label: "Title size",
    hint: "Pixels, from 1 to 200",
    rule: { min: 1, max: 200, integer: true },
  },
  {
    kind: "number",
    key: "valueSize",
    label: "Value size",
    hint: "Pixels, from 1 to 200",
    rule: { min: 1, max: 200, integer: true },
  },
]
