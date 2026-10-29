import { styleForSlot } from "../colors"
import {
  displayValue,
  type FieldConfig,
  type FieldResolver,
} from "../field-config"
import type { PreparedField, PreparedFrame } from "../transforms"

type FlameNode = {
  index: number
  label: string
  level: number
  value: number
  self: number
  parent: number
  start: number
  share: number
  color: string
}

export type FlameProfile = {
  nodes: FlameNode[]
  levels: number[][]
  total: number
  config: FieldConfig
  diff: boolean
}

export type FlameModel = { reason: string } | (FlameProfile & { reason: null })

export type FlameRange = { start: number; end: number }

export const missingFieldsReason =
  "A flame graph needs level, value, self and label fields in depth-first order."
export const brokenOrderReason =
  "The rows are not in depth-first order: a level jumps down by more than one."
export const diffNote = "Diff profiles show the left side."

const hueCount = 8
const separators = [".", "/", "::"]

function findField(frame: PreparedFrame, name: string) {
  const wanted = name.toLowerCase()
  return frame.fields.find((field) => field.name.toLowerCase() === wanted)
}

function hasField(frame: PreparedFrame, name: string): boolean {
  return findField(frame, name) !== undefined
}

export function packagePrefix(label: string): string {
  const cut = Math.max(...separators.map((token) => label.lastIndexOf(token)))
  return cut > 0 ? label.slice(0, cut) : label
}

function hashSlot(text: string): number {
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0) % hueCount
}

export function colorOf(label: string): string {
  return styleForSlot(hashSlot(packagePrefix(label))).color
}

function amount(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0
}

function levelOf(value: unknown): number | null {
  return typeof value === "number" && Number.isInteger(value) && value >= 0
    ? value
    : null
}

type Columns = {
  frame: PreparedFrame
  level: PreparedField
  value: PreparedField
  self: PreparedField
  label: PreparedField
}

function columnsOf(frames: PreparedFrame[]): Columns | null {
  for (const frame of frames) {
    const level = findField(frame, "level")
    const value = findField(frame, "value")
    const self = findField(frame, "self")
    const label = findField(frame, "label")
    if (level && value && self && label) {
      return { frame, level, value, self, label }
    }
  }
  return null
}

export function flameModel(
  frames: PreparedFrame[],
  fields: FieldResolver
): FlameModel {
  const columns = columnsOf(frames)
  if (!columns) return { reason: missingFieldsReason }
  const { frame, level, value, self, label } = columns
  const count = Math.min(
    level.values.length,
    value.values.length,
    self.values.length,
    label.values.length
  )
  const nodes: FlameNode[] = []
  const levels: number[][] = []
  const cursors: number[] = []
  const colors = new Map<string, string>()
  const stack: number[] = []
  let rootCursor = 0
  for (let index = 0; index < count; index++) {
    const depth = levelOf(level.values[index])
    if (depth === null || depth > stack.length) {
      return { reason: brokenOrderReason }
    }
    stack.length = depth
    const text = String(label.values[index] ?? "")
    const total = amount(value.values[index])
    const parent = depth === 0 ? -1 : stack[depth - 1]
    const start = parent === -1 ? rootCursor : cursors[parent]
    if (parent === -1) rootCursor += total
    else cursors[parent] += total
    let color = colors.get(text)
    if (color === undefined) {
      color = colorOf(text)
      colors.set(text, color)
    }
    nodes.push({
      index,
      label: text,
      level: depth,
      value: total,
      self: amount(self.values[index]),
      parent,
      start,
      share: 0,
      color,
    })
    cursors.push(start)
    stack.push(index)
    ;(levels[depth] ??= []).push(index)
  }
  if (rootCursor > 0) {
    for (const node of nodes) node.share = node.value / rootCursor
  }
  return {
    reason: null,
    nodes,
    levels,
    total: rootCursor,
    config: fields.configFor(frame, value),
    diff: hasField(frame, "valueRight") && hasField(frame, "selfRight"),
  }
}

export function fullRange(profile: FlameProfile): FlameRange {
  return { start: 0, end: profile.total }
}

export function zoomTo(profile: FlameProfile, nodeIndex: number): FlameRange {
  const node = profile.nodes[nodeIndex]
  if (!node || node.value <= 0) return fullRange(profile)
  return { start: node.start, end: node.start + node.value }
}

export function nodeAt(
  profile: FlameProfile,
  range: FlameRange,
  level: number,
  fraction: number
): number | null {
  const row = profile.levels[level]
  if (!row || row.length === 0) return null
  const position = range.start + fraction * (range.end - range.start)
  let low = 0
  let high = row.length - 1
  let found = -1
  while (low <= high) {
    const middle = (low + high) >> 1
    if (profile.nodes[row[middle]].start <= position) {
      found = middle
      low = middle + 1
    } else {
      high = middle - 1
    }
  }
  if (found === -1) return null
  const node = profile.nodes[row[found]]
  return position < node.start + node.value ? node.index : null
}

export function search(
  profile: FlameProfile,
  text: string
): ReadonlySet<number> | null {
  const needle = text.trim().toLowerCase()
  if (needle === "") return null
  const matches = new Set<number>()
  for (const node of profile.nodes) {
    if (node.label.toLowerCase().includes(needle)) matches.add(node.index)
  }
  return matches
}

export function shareText(share: number): string {
  return `${(share * 100).toFixed(2)}%`
}

const flameKeys = ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home"]

export type FlameKey =
  "ArrowUp" | "ArrowDown" | "ArrowLeft" | "ArrowRight" | "Home"

export function isFlameKey(key: string): key is FlameKey {
  return flameKeys.includes(key)
}

export function stepFocus(
  profile: FlameProfile,
  index: number | null,
  key: FlameKey
): number | null {
  const root = profile.levels[0]?.[0] ?? null
  const node = index === null ? undefined : profile.nodes[index]
  if (!node || key === "Home") return root
  switch (key) {
    case "ArrowUp":
      return node.parent === -1 ? node.index : node.parent
    case "ArrowDown":
      return (
        profile.levels[node.level + 1]?.find(
          (child) => profile.nodes[child].parent === node.index
        ) ?? node.index
      )
    case "ArrowLeft":
    case "ArrowRight": {
      const row = profile.levels[node.level]
      const next = row.indexOf(node.index) + (key === "ArrowLeft" ? -1 : 1)
      return row[next] ?? node.index
    }
  }
}

export function focusText(profile: FlameProfile, index: number): string {
  const node = profile.nodes[index]
  const total = displayValue(node.value, profile.config).text
  const self = displayValue(node.self, profile.config).text
  return `${node.label}, total ${total} (${shareText(node.share)}), self ${self}, level ${node.level + 1}`
}

export function zoomForFocus(
  profile: FlameProfile,
  zoomed: number | null,
  focused: number
): number | null {
  const node = profile.nodes[focused]
  if (!node || node.value <= 0) return zoomed
  const range = zoomed === null ? fullRange(profile) : zoomTo(profile, zoomed)
  if (node.start < range.end && node.start + node.value > range.start) {
    return zoomed
  }
  let ancestor = node.parent
  while (ancestor !== -1) {
    const candidate = profile.nodes[ancestor]
    if (
      candidate.parent !== -1 &&
      candidate.start <= range.start &&
      candidate.start + candidate.value >= range.end
    ) {
      return ancestor
    }
    ancestor = candidate.parent
  }
  return null
}
