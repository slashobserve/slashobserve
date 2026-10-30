import type { FieldConfig } from "../field-config"
import type { Cell } from "./stack"

export function transformValues(
  values: readonly Cell[],
  kind: FieldConfig["valueTransform"]
): Cell[] {
  if (kind === "negativeY") {
    return values.map((value) => (typeof value === "number" ? -value : value))
  }
  if (kind === "constant") {
    const first = values.find(
      (value): value is number =>
        typeof value === "number" && Number.isFinite(value)
    )
    return first === undefined ? [...values] : values.map(() => first)
  }
  return [...values]
}
