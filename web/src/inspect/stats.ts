import type { PreparedFrame } from "@/panels/transforms/names"

export type FrameStats = {
  frames: number
  series: number
  points: number
  rows: number
  minTime: number | null
  maxTime: number | null
}

function present(value: unknown): boolean {
  return value !== null && value !== undefined
}

export function frameStats(frames: readonly PreparedFrame[]): FrameStats {
  const stats: FrameStats = {
    frames: frames.length,
    series: 0,
    points: 0,
    rows: 0,
    minTime: null,
    maxTime: null,
  }
  for (const frame of frames) {
    let frameRows = 0
    for (const field of frame.fields) {
      frameRows = Math.max(frameRows, field.values.length)
      if (field.type === "number") {
        stats.series += 1
        stats.points += field.values.filter(present).length
      }
      if (field.type !== "time") continue
      for (const value of field.values) {
        if (typeof value !== "number" || !Number.isFinite(value)) continue
        if (stats.minTime === null || value < stats.minTime)
          stats.minTime = value
        if (stats.maxTime === null || value > stats.maxTime)
          stats.maxTime = value
      }
    }
    stats.rows += frameRows
  }
  return stats
}

function escapedName(name: string): string {
  return name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

export function queryVariables(
  text: string,
  names: readonly string[]
): string[] {
  return names.filter((name) => {
    const n = escapedName(name)
    return new RegExp(
      `\\$${n}(?![A-Za-z0-9_])|\\$\\{${n}(?::[^}]*)?\\}|\\[\\[${n}(?::[^\\]]*)?\\]\\]`
    ).test(text)
  })
}
