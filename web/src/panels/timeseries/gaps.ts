import type { Cell } from "./stack"

export function spanShortGaps(
  times: readonly number[],
  values: readonly Cell[],
  maxGapMs: number
): Cell[] {
  let out: Cell[] | null = null
  let before = -1
  let runStart = 0
  let sawNull = false
  for (let at = 0; at < values.length; at++) {
    const value = values[at]
    if (value === null) {
      sawNull = true
    } else if (typeof value === "number") {
      const joinable =
        before >= 0 && sawNull && (times[at] - times[before]) * 1000 <= maxGapMs
      if (joinable) {
        out ??= [...values]
        for (let fill = runStart; fill < at; fill++) out[fill] = undefined
      }
      before = at
      runStart = at + 1
      sawNull = false
    }
  }
  return out ?? (values as Cell[])
}
