import type { Reducer } from "@/spec"

import { isNumber, reduce } from "../reducers"
import { joinByField } from "./join"
import {
  cell,
  copyFrame,
  matchesName,
  rowCount,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type CalculateOptions = {
  alias: string
  expr?: string
  reduceRow?: Reducer
}

type Operand = { constant: number } | { field: PreparedField }
type Operator = "+" | "-" | "*" | "/"

const numberLiteral = /^[+-]?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i

function operandOf(frame: PreparedFrame, text: string): Operand | undefined {
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

function fromExpression(
  frame: PreparedFrame,
  expr: string
): unknown[] | undefined {
  const trimmed = expr.trim()
  for (const { left, op, right } of candidateSplits(trimmed)) {
    const a = operandOf(frame, left)
    const b = operandOf(frame, right)
    if (!a || !b) continue
    const values: unknown[] = []
    const count = rowCount(frame)
    for (let i = 0; i < count; i++) {
      values.push(apply(op, valueOf(a, i), valueOf(b, i)))
    }
    return values
  }
  return undefined
}

function fromRows(
  frame: PreparedFrame,
  reducer: Reducer
): unknown[] | undefined {
  const numbers = frame.fields.filter((field) => field.type === "number")
  if (numbers.length === 0) return undefined
  const values: unknown[] = []
  const count = rowCount(frame)
  for (let i = 0; i < count; i++) {
    values.push(
      reduce(
        numbers.map((field) => cell(field, i)),
        reducer
      )
    )
  }
  return values
}

function firstTimeName(frame: PreparedFrame): string | undefined {
  return frame.fields.find((field) => field.type === "time")?.name
}

function rightIsFixed(frames: PreparedFrame[], expr: string): boolean {
  for (const { left, right } of candidateSplits(expr.trim())) {
    const resolves = (text: string) =>
      frames.some((frame) => operandOf(frame, text) !== undefined)
    if (resolves(left) && resolves(right)) return numberLiteral.test(right)
  }
  return false
}

function joinsFirst(frames: PreparedFrame[], expr: string): boolean {
  if (frames.length < 2) return false
  const names = frames.map(firstTimeName)
  if (names.some((name) => name === undefined)) return false
  if (names.some((name) => name !== names[0])) return false
  return !rightIsFixed(frames, expr)
}

export function calculateField(
  frames: PreparedFrame[],
  { alias, expr, reduceRow }: CalculateOptions
): PreparedFrame[] {
  const input =
    expr !== undefined && joinsFirst(frames, expr)
      ? joinByField(frames, {})
      : frames
  return input.map((frame) => {
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
