import type { WireFrame } from "@/dashboards/queries"
import type { Reducer } from "@/spec"

import type { WireField } from "../frames"
import { reduce } from "../reducers"
import {
  cell,
  copyFrame,
  isNumber,
  matchesName,
  rowCount,
  withFields,
} from "./names"

export type CalculateOptions = {
  alias: string
  expr?: string
  reduceRow?: Reducer
}

type Operand = { constant: number } | { field: WireField }
type Operator = "+" | "-" | "*" | "/"

const numberLiteral = /^[+-]?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i

function operandOf(frame: WireFrame, text: string): Operand | undefined {
  if (numberLiteral.test(text)) return { constant: Number(text) }
  const field = frame.fields.find(
    (candidate) =>
      candidate.type === "number" && matchesName(frame, candidate, text)
  )
  return field ? { field } : undefined
}

type Split = { left: string; op: Operator; right: string }

function isOperator(char: string): char is Operator {
  return char === "+" || char === "-" || char === "*" || char === "/"
}

function candidateSplits(expr: string): Split[] {
  const spaced: Split[] = []
  const tight: Split[] = []
  for (let i = 1; i < expr.length - 1; i++) {
    const char = expr[i]
    if (!isOperator(char)) continue
    const split = {
      left: expr.slice(0, i).trim(),
      op: char,
      right: expr.slice(i + 1).trim(),
    }
    if (split.left === "" || split.right === "") continue
    const isSpaced = /\s/.test(expr[i - 1]) && /\s/.test(expr[i + 1])
    if (isSpaced) spaced.push(split)
    else tight.push(split)
  }
  return [...spaced, ...tight]
}

function apply(op: Operator, a: unknown, b: unknown): number | null {
  if (!isNumber(a) || !isNumber(b)) return null
  let result: number
  switch (op) {
    case "+":
      result = a + b
      break
    case "-":
      result = a - b
      break
    case "*":
      result = a * b
      break
    case "/":
      if (b === 0) return null
      result = a / b
      break
  }
  return Number.isFinite(result) ? result : null
}

function valueOf(operand: Operand, row: number): unknown {
  return "constant" in operand ? operand.constant : cell(operand.field, row)
}

function fromExpression(frame: WireFrame, expr: string): unknown[] | undefined {
  const trimmed = expr.trim()
  for (const { left, op, right } of candidateSplits(trimmed)) {
    const a = operandOf(frame, left)
    const b = operandOf(frame, right)
    if (!a || !b) continue
    const values: unknown[] = []
    for (let i = 0; i < rowCount(frame); i++) {
      values.push(apply(op, valueOf(a, i), valueOf(b, i)))
    }
    return values
  }
  return undefined
}

function fromRows(frame: WireFrame, reducer: Reducer): unknown[] | undefined {
  const numbers = frame.fields.filter((field) => field.type === "number")
  if (numbers.length === 0) return undefined
  const values: unknown[] = []
  for (let i = 0; i < rowCount(frame); i++) {
    values.push(
      reduce(
        numbers.map((field) => cell(field, i)),
        reducer
      )
    )
  }
  return values
}

export function calculateField(
  frames: WireFrame[],
  { alias, expr, reduceRow }: CalculateOptions
): WireFrame[] {
  return frames.map((frame) => {
    const values =
      expr !== undefined
        ? fromExpression(frame, expr)
        : reduceRow !== undefined
          ? fromRows(frame, reduceRow)
          : undefined
    if (!values) return copyFrame(frame)
    return withFields(frame, [
      ...frame.fields.filter((field) => field.name !== alias),
      { name: alias, type: "number", values },
    ])
  })
}
