export type StackMode = "none" | "normal" | "percent"

export type Cell = number | null | undefined

export type Direction = 1 | -1

export type Stacked = {
  drawn: Cell[][]
  below: (number | null)[]
  directions: Direction[]
}

const directionSamples = 100

export function stackDirection(values: readonly Cell[]): Direction {
  let first = 0
  let last = values.length - 1
  while (first <= last && values[first] == null) first++
  while (last >= first && values[last] == null) last--
  if (last < first) return 1
  const stride = Math.max(1, Math.floor((last - first + 1) / directionSamples))
  let negative = 0
  let positive = 0
  for (let at = first; at <= last; at += stride) {
    const value = values[at]
    if (typeof value !== "number") continue
    if (value < 0 || Object.is(value, -0)) negative++
    else if (value > 0) positive++
  }
  return negative > positive ? -1 : 1
}

type Group = { members: number[]; mode: StackMode; direction: Direction }

function stackGroups(
  values: readonly (readonly Cell[])[],
  modes: readonly StackMode[],
  axes: readonly string[]
): Group[] {
  const groups = new Map<string, Group>()
  modes.forEach((mode, index) => {
    if (mode === "none") return
    const direction = stackDirection(values[index])
    const key = `${direction}\u0000${mode}\u0000${axes[index]}`
    const group = groups.get(key)
    if (group) group.members.push(index)
    else groups.set(key, { members: [index], mode, direction })
  })
  return [...groups.values()]
}

export function stackSeries(
  values: readonly (readonly Cell[])[],
  modes: readonly StackMode[],
  axes: readonly string[]
): Stacked {
  const drawn: Cell[][] = values.map((series) => [...series])
  const below = modes.map(() => null as number | null)
  const directions = modes.map((): Direction => 1)
  for (const { members, mode, direction } of stackGroups(values, modes, axes)) {
    const length = values[members[0]].length
    const sums = new Array<number>(length).fill(0)
    const totals = new Array<number>(length).fill(0)
    for (const index of members) {
      values[index].forEach((value, at) => {
        totals[at] += value ?? 0
      })
    }
    members.forEach((index, position) => {
      below[index] = position === 0 ? null : members[position - 1]
      directions[index] = direction
      drawn[index] = values[index].map((value, at) => {
        sums[at] += value ?? 0
        if (value == null) return value
        if (mode === "normal") return sums[at]
        return totals[at] === 0 ? 0 : direction * (sums[at] / totals[at])
      })
    })
  }
  return { drawn, below, directions }
}
