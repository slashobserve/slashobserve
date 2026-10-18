const hueCount = 8
const longDash = [6, 4]
const shortDash = [2, 3]

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
}

export function readChartColors() {
  return {
    grid: cssVar("--chart-grid"),
    axis: cssVar("--chart-axis"),
    muted: cssVar("--muted-foreground"),
    font: getComputedStyle(document.documentElement).fontFamily,
  }
}

export function resolveColor(color: string): string {
  const match = /^var\((--[\w-]+)\)$/.exec(color)
  return match ? cssVar(match[1]) : color
}

export type SlotMap = ReadonlyMap<string, number>
export type LineStyle = "solid" | "dashed" | "dotted"
export type SeriesStyle = { color: string; dash?: number[]; line: LineStyle }

export function assignSlots(slots: SlotMap, keys: readonly string[]): SlotMap {
  const active = new Set(keys)
  const taken = new Set<number>()
  for (const key of keys) {
    const slot = slots.get(key)
    if (slot !== undefined) taken.add(slot)
  }
  let next: Map<string, number> | null = null
  for (const key of keys) {
    if ((next ?? slots).has(key)) continue
    next ??= new Map(slots)
    let slot = 0
    while (taken.has(slot)) slot++
    for (const [other, otherSlot] of next) {
      if (otherSlot === slot && !active.has(other)) next.delete(other)
    }
    next.set(key, slot)
    taken.add(slot)
  }
  return next ?? slots
}

export function styleForSlot(slot: number): SeriesStyle {
  const color = `var(--series-${(slot % hueCount) + 1})`
  if (slot < hueCount) return { color, line: "solid" }
  if (slot < 2 * hueCount) return { color, dash: longDash, line: "dashed" }
  return { color, dash: shortDash, line: "dotted" }
}
