import type { LonLat } from "./projection"

const coordinate = String.raw`[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`
const point = new RegExp(
  String.raw`^\s*(?:SRID=\d+;\s*)?POINT\s*\(\s*(${coordinate})\s+(${coordinate})\s*\)\s*$`,
  "i"
)

export function parseWktPoint(text: unknown): LonLat | null {
  if (typeof text !== "string") return null
  const match = point.exec(text)
  if (!match) return null
  return { lon: Number(match[1]), lat: Number(match[2]) }
}
