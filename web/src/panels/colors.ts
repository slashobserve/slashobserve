const hueCount = 8
const longDash = [6, 4]
const shortDash = [2, 3]

const tokenCache = { scope: "", values: new Map<string, string>() }

function cachedTokens(): Map<string, string> {
  const scope = document.documentElement.className
  if (scope !== tokenCache.scope) {
    tokenCache.scope = scope
    tokenCache.values = new Map()
  }
  return tokenCache.values
}

function cachedRead(
  key: string,
  read: (style: CSSStyleDeclaration) => string
): string {
  const values = cachedTokens()
  let value = values.get(key)
  if (value === undefined) {
    value = read(getComputedStyle(document.documentElement))
    if (value !== "") values.set(key, value)
  }
  return value
}

function cssVar(name: string): string {
  return cachedRead(name, (style) => style.getPropertyValue(name).trim())
}

const fontFamilyKey = "font-family"

export function readChartColors() {
  return {
    grid: cssVar("--chart-grid"),
    muted: cssVar("--muted-foreground"),
    font: cachedRead(fontFamilyKey, (style) => style.fontFamily),
  }
}

export function resolveColor(color: string): string {
  return color.replace(/var\((--[\w-]+)\)/g, (_, name: string) => cssVar(name))
}

export function hashSlot(text: string): number {
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0) % hueCount
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
