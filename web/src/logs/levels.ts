import type { LogLevelKind } from "@/panels/logs/logs-model"

import { volumeKinds } from "./volume"

export type ShownLevels = readonly LogLevelKind[] | undefined

function ordered(kinds: ReadonlySet<LogLevelKind>): LogLevelKind[] {
  return volumeKinds.filter((kind) => kinds.has(kind))
}

function normalized(kinds: ReadonlySet<LogLevelKind>): ShownLevels {
  const list = ordered(kinds)
  return list.length === 0 || list.length === volumeKinds.length
    ? undefined
    : list
}

export function parseLevels(value: unknown): ShownLevels {
  const text = Array.isArray(value) ? value[0] : value
  if (typeof text !== "string") return undefined
  const named = new Set(text.split(","))
  return normalized(new Set(volumeKinds.filter((kind) => named.has(kind))))
}

export function levelsParam(shown: ShownLevels): string | undefined {
  return shown === undefined ? undefined : shown.join(",")
}

export function hiddenLevels(shown: ShownLevels): ReadonlySet<LogLevelKind> {
  return shown === undefined
    ? new Set()
    : new Set(volumeKinds.filter((kind) => !shown.includes(kind)))
}

export function pickLevel(
  shown: ShownLevels,
  kind: LogLevelKind,
  additive: boolean
): ShownLevels {
  if (!additive) {
    return shown?.length === 1 && shown[0] === kind ? undefined : [kind]
  }
  const next = new Set(shown ?? volumeKinds)
  if (!next.delete(kind)) next.add(kind)
  return normalized(next)
}
