import type { WireFrame } from "@/dashboards/queries"

import type { WireField } from "../frames"
import { cell, copyFrame, isNumber, matchesName, rowCount } from "./names"

export type SortOptions = { field: string; desc?: boolean }

function compare(a: unknown, b: unknown): number {
  if (isNumber(a) && isNumber(b)) return a - b
  if (typeof a === "boolean" && typeof b === "boolean") {
    return Number(a) - Number(b)
  }
  const text = (value: unknown) =>
    typeof value === "string" ? value : JSON.stringify(value)
  return text(a).localeCompare(text(b))
}

function sortFrame(frame: WireFrame, { field, desc }: SortOptions): WireFrame {
  const key: WireField | undefined = frame.fields.find((candidate) =>
    matchesName(frame, candidate, field)
  )
  if (!key) return copyFrame(frame)
  const order = Array.from({ length: rowCount(frame) }, (_, index) => index)
  order.sort((a, b) => {
    const left = cell(key, a)
    const right = cell(key, b)
    if (left === null || right === null) {
      return left === right ? 0 : left === null ? 1 : -1
    }
    return desc ? compare(right, left) : compare(left, right)
  })
  return {
    ...frame,
    fields: frame.fields.map((candidate) => ({
      ...candidate,
      values: order.map((row) => cell(candidate, row)),
    })),
  }
}

export function sortBy(frames: WireFrame[], options: SortOptions): WireFrame[] {
  return frames.map((frame) => sortFrame(frame, options))
}
