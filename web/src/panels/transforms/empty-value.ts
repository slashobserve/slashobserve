import type { Transform } from "@/spec"

import type { ColumnType } from "../frames"
import { isNumber } from "../reducers"

type EmptyValue = NonNullable<
  Extract<Transform, { type: "transpose" }>["empty"]
>

const fills: Record<EmptyValue, unknown> = {
  empty: "",
  zero: 0,
  true: true,
  false: false,
}

export function fillValue(empty: EmptyValue | undefined): unknown {
  return empty === undefined ? null : fills[empty]
}

export function commonType(values: readonly unknown[]): ColumnType {
  const present = values.filter(
    (value) => value !== null && value !== undefined
  )
  if (present.length === 0) return "string"
  if (present.every(isNumber)) return "number"
  if (present.every((value) => typeof value === "boolean")) return "bool"
  if (present.every((value) => typeof value === "string")) return "string"
  return "other"
}
