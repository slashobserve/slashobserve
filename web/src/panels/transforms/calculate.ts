import type { Reducer, Transform } from "@/spec"

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

type CalculateTransform = Extract<Transform, { type: "calculateField" }>

export type CalculateOptions = Pick<
  CalculateTransform,
  | "alias"
  | "expr"
  | "reduceRow"
  | "unary"
  | "index"
  | "cumulative"
  | "window"
  | "replace"
>

type Operand = { constant: number } | { field: PreparedField }
type Operator = "+" | "-" | "*" | "/" | "%" | "^"

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
  return "+-*/%^".includes(char)
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
    case "%":
      if (b === 0) return null
      result = a % b
      break
    case "^":
      result = a ** b
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

function firstNumberField(
  frame: PreparedFrame,
  name: string | undefined
): PreparedField | undefined {
  return frame.fields.find(
    (field) =>
      field.type === "number" &&
      (name === undefined || matchesName(frame, field, name))
  )
}

type Unary = NonNullable<CalculateOptions["unary"]>
type RowIndex = NonNullable<CalculateOptions["index"]>
type Cumulative = NonNullable<CalculateOptions["cumulative"]>
type Window = NonNullable<CalculateOptions["window"]>

const unaryFunctions: Record<
  NonNullable<Unary["fn"]>,
  (value: number) => number
> = {
  abs: Math.abs,
  exp: Math.exp,
  ln: Math.log,
  floor: Math.floor,
  ceil: Math.ceil,
  round: Math.round,
}

function fromUnary(
  frame: PreparedFrame,
  { fn = "abs", field }: Unary
): unknown[] | undefined {
  const source = firstNumberField(frame, field)
  if (!source) return undefined
  const apply = unaryFunctions[fn]
  return source.values.map((value) => {
    if (!isNumber(value)) return null
    const result = apply(value)
    return Number.isFinite(result) ? result : null
  })
}

function fromIndex(frame: PreparedFrame, { percentile }: RowIndex): unknown[] {
  const count = rowCount(frame)
  return Array.from({ length: count }, (_, row) =>
    percentile ? (count > 1 ? row / (count - 1) : 0) : row
  )
}

function fromCumulative(
  frame: PreparedFrame,
  { fn = "sum", field }: Cumulative
): unknown[] | undefined {
  const source = firstNumberField(frame, field)
  if (!source) return undefined
  let total = 0
  let seen = 0
  return source.values.map((value) => {
    if (!isNumber(value)) return null
    total += value
    seen += 1
    return fn === "mean" ? total / seen : total
  })
}

const defaultWindowShare = 0.1

function windowWidth(count: number, { size, share }: Window): number {
  if (size !== undefined) return size
  return Math.max(1, Math.round((share ?? defaultWindowShare) * count))
}

function fromWindow(
  frame: PreparedFrame,
  options: Window
): unknown[] | undefined {
  const { fn = "mean", field, align = "trailing" } = options
  const source = firstNumberField(frame, field)
  if (!source) return undefined
  const count = source.values.length
  const width = windowWidth(count, options)
  const numbers = source.values.filter(isNumber)
  const shift =
    numbers.length === 0
      ? 0
      : numbers.reduce((total, value) => total + value, 0) / numbers.length
  const seen = [0]
  const sums = [0]
  const squares = [0]
  for (const value of source.values) {
    const centered = isNumber(value) ? value - shift : 0
    seen.push(seen[seen.length - 1] + (isNumber(value) ? 1 : 0))
    sums.push(sums[sums.length - 1] + centered)
    squares.push(squares[squares.length - 1] + centered * centered)
  }
  return source.values.map((value, row) => {
    if (!isNumber(value)) return null
    const first =
      align === "centered" ? row - Math.floor(width / 2) : row - width + 1
    const start = Math.max(0, first)
    const end = Math.min(count, first + width)
    const members = seen[end] - seen[start]
    if (members === 0) return null
    const mean = (sums[end] - sums[start]) / members
    const variance = Math.max(
      0,
      (squares[end] - squares[start]) / members - mean * mean
    )
    switch (fn) {
      case "mean":
        return mean + shift
      case "variance":
        return variance
      case "stdDev":
        return Math.sqrt(variance)
    }
  })
}

function valuesFor(
  frame: PreparedFrame,
  options: CalculateOptions
): unknown[] | undefined {
  if (options.expr !== undefined) return fromExpression(frame, options.expr)
  if (options.reduceRow !== undefined) {
    return fromRows(frame, options.reduceRow)
  }
  if (options.unary) return fromUnary(frame, options.unary)
  if (options.index) return fromIndex(frame, options.index)
  if (options.cumulative) return fromCumulative(frame, options.cumulative)
  if (options.window) return fromWindow(frame, options.window)
  return undefined
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
  options: CalculateOptions
): PreparedFrame[] {
  const { alias, expr, replace } = options
  const input =
    expr !== undefined && joinsFirst(frames, expr)
      ? joinByField(frames, {})
      : frames
  return input.map((frame) => {
    const values = valuesFor(frame, options)
    if (!values) return copyFrame(frame)
    const calculated: PreparedField = { name: alias, type: "number", values }
    const time = frame.fields.find((field) => field.type === "time")
    return withFields(
      frame,
      replace
        ? [...(time ? [time] : []), calculated]
        : [...frame.fields.filter((field) => field.name !== alias), calculated]
    )
  })
}
