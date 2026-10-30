import type { LonLat } from "./projection"

type Bounds = { west: number; south: number; east: number; north: number }

export const regionBounds: Record<string, Bounds> = {
  "north-america": { west: -170, south: 7, east: -50, north: 72 },
  "south-america": { west: -82, south: -56, east: -34, north: 13 },
  europe: { west: -25, south: 34, east: 45, north: 72 },
  africa: { west: -18, south: -35, east: 52, north: 38 },
  "west-asia": { west: 25, south: 12, east: 63, north: 43 },
  "south-asia": { west: 60, south: 5, east: 98, north: 38 },
  "south-east-asia": { west: 92, south: -11, east: 141, north: 28 },
  "east-asia": { west: 73, south: 18, east: 146, north: 54 },
  australia: { west: 112, south: -44, east: 154, north: -10 },
  oceania: { west: 110, south: -48, east: 180, north: 20 },
}

export function isRegion(value: unknown): value is string {
  return typeof value === "string" && Object.hasOwn(regionBounds, value)
}

export function regionCorners(region: string): LonLat[] {
  const { west, south, east, north } = regionBounds[region]
  return [
    { lon: west, lat: south },
    { lon: east, lat: north },
  ]
}
