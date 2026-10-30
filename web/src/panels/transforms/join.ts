import { uniqueKey } from "../frames"
import {
  cell,
  compareValues,
  copyFrame,
  rowCount,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type JoinMode = "outer" | "inner" | "outerTabular"
export type JoinOptions = { field?: string; mode?: JoinMode }

type Member = { frame: PreparedFrame; key: PreparedField }
type Joined = { key: unknown; rowOf: number[] }

function firstTime(frame: PreparedFrame): PreparedField | undefined {
  return frame.fields.find((field) => field.type === "time")
}

function namedKey(
  frame: PreparedFrame,
  name: string
): PreparedField | undefined {
  return frame.fields.find(
    (field) => field.name === name || seriesName(frame, field) === name
  )
}

function commonNumberKey(frames: PreparedFrame[]): string | undefined {
  const candidate = frames[0]?.fields.find((field) => field.type === "number")
  if (!candidate) return undefined
  const everywhere = frames.every((frame) =>
    frame.fields.some(
      (field) => field.type === "number" && field.name === candidate.name
    )
  )
  return everywhere ? candidate.name : undefined
}

function memberOf(
  frames: PreparedFrame[],
  field: string | undefined
): (frame: PreparedFrame) => PreparedField | undefined {
  if (field !== undefined) return (frame) => namedKey(frame, field)
  if (frames[0] && !firstTime(frames[0])) {
    const shared = commonNumberKey(frames)
    return (frame) =>
      shared === undefined
        ? undefined
        : frame.fields.find(
            (candidate) =>
              candidate.type === "number" && candidate.name === shared
          )
  }
  return firstTime
}

function keyFirst(
  frame: PreparedFrame,
  key: PreparedField,
  sorted: boolean
): PreparedFrame {
  const rows = rowCount(frame)
  const order = Array.from({ length: rows }, (_, index) => index)
  if (sorted) {
    order.sort((a, b) => {
      const left = cell(key, a)
      const right = cell(key, b)
      if (left === null || right === null) {
        return left === right ? 0 : left === null ? 1 : -1
      }
      return compareValues(left, right)
    })
  }
  const reorder = (field: PreparedField): PreparedField => ({
    ...field,
    values: order.map((row) => cell(field, row)),
  })
  return {
    ...frame,
    fields: [
      reorder(key),
      ...frame.fields.filter((field) => field !== key).map(reorder),
    ],
  }
}

type Column = { member: number; source: PreparedField; out: PreparedField }

function outputColumns(members: Member[]): Column[] {
  const [head] = members
  const taken = new Map<string, number>()
  for (const { frame, key } of members) {
    for (const field of frame.fields) {
      if (field === key) continue
      taken.set(field.name, (taken.get(field.name) ?? 0) + 1)
    }
  }
  const used = new Map<string, number>()
  uniqueKey(used, head.key.name)
  const columns: Column[] = []
  members.forEach(({ frame, key }, member) => {
    for (const field of frame.fields) {
      if (field === key) continue
      const series = seriesName(frame, field)
      const shared = (taken.get(field.name) ?? 0) > 1
      columns.push({
        member,
        source: field,
        out: {
          name: uniqueKey(used, shared ? series : field.name),
          type: field.type,
          values: [],
          seriesName: series,
        },
      })
    }
  })
  return columns
}

function keyedRows(members: Member[], mode: "outer" | "inner"): Joined[] {
  const byId = new Map<string, Joined>()
  const keyed: Joined[] = []
  const unkeyed: Joined[] = []
  members.forEach(({ frame, key }, member) => {
    const seen = new Map<string, number>()
    const rows = rowCount(frame)
    for (let row = 0; row < rows; row++) {
      const value = cell(key, row)
      if (value === null) {
        const joined = { key: null, rowOf: members.map(() => -1) }
        joined.rowOf[member] = row
        unkeyed.push(joined)
        continue
      }
      const base = `${typeof value}:${String(value)}`
      const occurrence = seen.get(base) ?? 0
      seen.set(base, occurrence + 1)
      const id = `${base}#${occurrence}`
      let joined = byId.get(id)
      if (!joined) {
        joined = { key: value, rowOf: members.map(() => -1) }
        byId.set(id, joined)
        keyed.push(joined)
      }
      joined.rowOf[member] = row
    }
  })

  const kept = (
    mode === "outer"
      ? keyed
      : keyed.filter((row) => row.rowOf.every((at) => at >= 0))
  ).sort((a, b) => compareValues(a.key, b.key))
  return mode === "outer" ? [...kept, ...unkeyed] : kept
}

function tabularRows(members: Member[]): Joined[] {
  const width = members.length
  const blank = () => Array<number>(width).fill(-1)
  const [head, ...rest] = members
  let rows: Joined[] = Array.from(
    { length: rowCount(head.frame) },
    (_, row) => {
      const rowOf = blank()
      rowOf[0] = row
      return { key: cell(head.key, row), rowOf }
    }
  )
  rest.forEach(({ frame, key }, offset) => {
    const member = offset + 1
    const byKey = new Map<string, number[]>()
    const count = rowCount(frame)
    for (let row = 0; row < count; row++) {
      const value = cell(key, row)
      if (value === null) continue
      const id = `${typeof value}:${String(value)}`
      byKey.set(id, [...(byKey.get(id) ?? []), row])
    }
    const matched = new Set<number>()
    const next: Joined[] = []
    for (const joined of rows) {
      const matches =
        joined.key === null
          ? undefined
          : byKey.get(`${typeof joined.key}:${String(joined.key)}`)
      if (!matches) {
        next.push(joined)
        continue
      }
      for (const row of matches) {
        matched.add(row)
        const rowOf = [...joined.rowOf]
        rowOf[member] = row
        next.push({ key: joined.key, rowOf })
      }
    }
    for (let row = 0; row < count; row++) {
      if (matched.has(row)) continue
      const rowOf = blank()
      rowOf[member] = row
      next.push({ key: cell(key, row), rowOf })
    }
    rows = next
  })
  return rows
}

function joinMembers(members: Member[], mode: JoinMode): PreparedFrame {
  const [head] = members
  const columns = outputColumns(members)
  const rows =
    mode === "outerTabular" ? tabularRows(members) : keyedRows(members, mode)
  return {
    ref: head.frame.ref,
    fields: [
      {
        name: head.key.name,
        type: head.key.type,
        values: rows.map((row) => row.key),
      },
      ...columns.map(({ member, source, out }) => ({
        ...out,
        values: rows.map((row) =>
          row.rowOf[member] < 0 ? null : cell(source, row.rowOf[member])
        ),
      })),
    ],
  }
}

export function joinByField(
  frames: PreparedFrame[],
  { field, mode = "outer" }: JoinOptions
): PreparedFrame[] {
  const keyOf = memberOf(frames, field)
  const members: Member[] = []
  const others: PreparedFrame[] = []
  for (const frame of frames) {
    const key = keyOf(frame)
    if (key) members.push({ frame, key })
    else others.push(frame)
  }
  if (members.length === 0) return frames.map(copyFrame)
  const joined =
    members.length === 1
      ? keyFirst(members[0].frame, members[0].key, mode !== "outerTabular")
      : joinMembers(members, mode)
  return [joined, ...others.map(copyFrame)]
}
