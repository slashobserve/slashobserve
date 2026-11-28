import type { Transform } from "@/spec"

import { isNumber } from "../reducers"
import {
  frameLabels,
  frameName,
  matchesName,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type RegressionOptions = Extract<Transform, { type: "regression" }>

const defaultDegree = 2
const pivotLimit = 1e-12

function augmented(matrix: number[][], target: number[]): number[][] | null {
  const rows: number[][] = []
  for (const [index, row] of matrix.entries()) {
    const value = target[index]
    if (value === undefined || row.length !== target.length) return null
    rows.push([...row, value])
  }
  return rows.length === target.length ? rows : null
}

function solve(matrix: number[][], target: number[]): number[] | null {
  const size = target.length
  const rows = augmented(matrix, target)
  if (!rows) return null
  const at = (row: number, column: number) => rows[row]?.[column] ?? Number.NaN
  for (let column = 0; column < size; column++) {
    let pivot = column
    for (let row = column + 1; row < size; row++) {
      if (Math.abs(at(row, column)) > Math.abs(at(pivot, column))) {
        pivot = row
      }
    }
    const pivotRow = rows[pivot]
    const columnRow = rows[column]
    if (!pivotRow || !columnRow) return null
    if (Math.abs(at(pivot, column)) < pivotLimit) return null
    rows[column] = pivotRow
    rows[pivot] = columnRow
    for (const current of rows.slice(column + 1)) {
      const factor = (current[column] ?? Number.NaN) / at(column, column)
      for (let index = column; index <= size; index++) {
        current[index] =
          (current[index] ?? Number.NaN) - factor * at(column, index)
      }
    }
  }
  const solution = new Array<number>(size).fill(0)
  for (let row = size - 1; row >= 0; row--) {
    let rest = at(row, size)
    for (let index = row + 1; index < size; index++) {
      rest -= at(row, index) * (solution[index] ?? Number.NaN)
    }
    solution[row] = rest / at(row, row)
  }
  return solution
}

function fitPolynomial(
  xs: readonly number[],
  ys: readonly number[],
  degree: number
): ((x: number) => number) | null {
  const order = Math.min(degree, xs.length - 1)
  if (order < 1) return null
  const low = xs.reduce((a, b) => Math.min(a, b))
  const high = xs.reduce((a, b) => Math.max(a, b))
  const scale = high - low || 1
  const unit = (x: number) => (x - low) / scale
  const power = (x: number, exponent: number) => unit(x) ** exponent
  const matrix: number[][] = []
  const target: number[] = []
  for (let row = 0; row <= order; row++) {
    matrix.push(
      Array.from({ length: order + 1 }, (_, column) =>
        xs.reduce((sum, x) => sum + power(x, row + column), 0)
      )
    )
    target.push(
      xs.reduce((sum, x, at) => sum + power(x, row) * (ys[at] ?? Number.NaN), 0)
    )
  }
  const coefficients = solve(matrix, target)
  if (!coefficients) return null
  return (x) =>
    coefficients.reduce((sum, c, exponent) => sum + c * power(x, exponent), 0)
}

function find(
  frame: PreparedFrame,
  name: string | undefined,
  fallback: (field: PreparedField) => boolean
): PreparedField | undefined {
  return name === undefined
    ? frame.fields.find(fallback)
    : frame.fields.find((field) => matchesName(frame, field, name))
}

function predictionXs(
  xs: readonly number[],
  predictions: number | undefined
): number[] {
  if (predictions === undefined) return [...xs]
  const low = xs.reduce((a, b) => Math.min(a, b))
  const high = xs.reduce((a, b) => Math.max(a, b))
  if (predictions === 1) return [low]
  return Array.from(
    { length: predictions },
    (_, at) => low + ((high - low) * at) / (predictions - 1)
  )
}

function regress(
  frame: PreparedFrame,
  { model, degree, x, y, predictions }: RegressionOptions
): PreparedFrame | null {
  const xField = find(
    frame,
    x,
    (field) => field.type === "time" || field.type === "number"
  )
  const yField = find(
    frame,
    y,
    (field) => field.type === "number" && field !== xField
  )
  if (!xField || !yField) return null
  const xs: number[] = []
  const ys: number[] = []
  yField.values.forEach((value, row) => {
    const xValue = xField.values[row]
    if (isNumber(xValue) && isNumber(value)) {
      xs.push(xValue)
      ys.push(value)
    }
  })
  if (xs.length < 2) return null
  const order = model === "polynomial" ? (degree ?? defaultDegree) : 1
  const predict = fitPolynomial(xs, ys, order)
  if (!predict) return null
  const predicted = predictionXs(xs, predictions).map((value) =>
    xField.type === "time" ? Math.round(value) : value
  )
  return {
    ref: frame.ref,
    ...frameName(frame),
    ...frameLabels(frame),
    fields: [
      { name: xField.name, type: xField.type, values: predicted },
      {
        name: `${yField.name} regression`,
        type: "number",
        seriesName: `${seriesName(frame, yField)} regression`,
        values: predicted.map(predict),
      },
    ],
  }
}

export function regression(
  frames: PreparedFrame[],
  options: RegressionOptions
): PreparedFrame[] {
  return frames.flatMap((frame) => {
    const fitted = regress(frame, options)
    return fitted ? [frame, fitted] : [frame]
  })
}
