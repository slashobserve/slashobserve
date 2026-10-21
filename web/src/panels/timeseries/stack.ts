export type StackMode = "none" | "normal" | "percent"

type Values = (number | null)[]

function stackGroups(
  modes: readonly StackMode[],
  axes: readonly string[]
): number[][] {
  const groups = new Map<string, number[]>()
  modes.forEach((mode, index) => {
    if (mode === "none") return
    const key = `${mode}\u0000${axes[index]}`
    const members = groups.get(key)
    if (members) members.push(index)
    else groups.set(key, [index])
  })
  return [...groups.values()]
}

export function stackSeries(
  values: readonly Values[],
  modes: readonly StackMode[],
  axes: readonly string[]
): Values[] {
  const drawn = [...values]
  for (const members of stackGroups(modes, axes)) {
    const length = values[members[0]].length
    const sums = new Array<number>(length).fill(0)
    const totals =
      modes[members[0]] === "percent"
        ? members.reduce<number[]>(
            (total, index) =>
              total.map((sum, at) => sum + (values[index][at] ?? 0)),
            new Array<number>(length).fill(0)
          )
        : null
    for (const index of members) {
      drawn[index] = values[index].map((value, at) => {
        sums[at] += value ?? 0
        if (value === null) return null
        if (totals === null) return sums[at]
        return totals[at] === 0 ? 0 : sums[at] / totals[at]
      })
    }
  }
  return drawn
}

export function stackBelow(
  modes: readonly StackMode[],
  axes: readonly string[]
): (number | null)[] {
  const below = modes.map(() => null as number | null)
  for (const members of stackGroups(modes, axes)) {
    members.forEach((index, position) => {
      below[index] = position === 0 ? null : members[position - 1]
    })
  }
  return below
}
