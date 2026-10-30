import {
  unwrapRing,
  type Feature,
  type Geometry,
  type Position,
} from "../layers/geojson"
import { project, unproject, type LonLat } from "../projection"
import type { AtlasName } from "../world"
import { countryAliases, isoCountries } from "./iso-countries"
import { usStateCodes } from "./us-states"

export type GazetteerName = AtlasName | "airports"

export type Gazetteer = { find: (value: unknown) => LonLat | null }

export type Gazetteers = Partial<Record<AtlasName, Gazetteer>>

export type LookupState =
  | { state: "ready"; gazetteer: Gazetteer }
  | { state: "pending" }
  | { state: "unavailable"; note: string }

export const airportsUnavailable = "Airport lookup is not available."

export function gazetteerName(value: unknown): GazetteerName {
  return value === "usa-states" || value === "airports" ? value : "countries"
}

export function atlasOf(name: GazetteerName): AtlasName | null {
  return name === "airports" ? null : name
}

export function resolveLookup(
  name: GazetteerName,
  loaded: Gazetteers
): LookupState {
  if (name === "airports") {
    return { state: "unavailable", note: airportsUnavailable }
  }
  const gazetteer = loaded[name]
  return gazetteer ? { state: "ready", gazetteer } : { state: "pending" }
}

type Ring = { area: number; x: number; y: number }

function ringCentroid(ring: readonly Position[]): Ring | null {
  const points = unwrapRing(ring).map(([lon, lat]) => project(lon, lat))
  let twiceArea = 0
  let sumX = 0
  let sumY = 0
  points.forEach((a, index) => {
    const b = points[(index + 1) % points.length]
    const cross = a.x * b.y - b.x * a.y
    twiceArea += cross
    sumX += (a.x + b.x) * cross
    sumY += (a.y + b.y) * cross
  })
  if (twiceArea === 0) return null
  return {
    area: Math.abs(twiceArea) / 2,
    x: sumX / (3 * twiceArea),
    y: sumY / (3 * twiceArea),
  }
}

function polygons(geometry: Geometry): (readonly Position[])[] {
  if (geometry.type === "Polygon") return [geometry.coordinates[0] ?? []]
  if (geometry.type === "MultiPolygon") {
    return geometry.coordinates.map((polygon) => polygon[0] ?? [])
  }
  return []
}

export function centroidOf(geometry: Geometry): LonLat | null {
  let largest: Ring | null = null
  for (const ring of polygons(geometry)) {
    const found = ringCentroid(ring)
    if (found && (largest === null || found.area > largest.area)) {
      largest = found
    }
  }
  if (!largest) return null
  const place = unproject(largest.x, largest.y)
  return { lat: place.lat, lon: wrapLon(place.lon) }
}

function wrapLon(lon: number): number {
  return ((((lon + 180) % 360) + 360) % 360) - 180
}

function normal(value: string): string {
  return value.trim().toLowerCase()
}

function codesOf(name: AtlasName, id: string | number | undefined): string[] {
  const key = String(id)
  if (name === "usa-states") {
    return Object.entries(usStateCodes)
      .filter(([, fips]) => fips === key)
      .map(([code]) => code)
  }
  const country = isoCountries.find(([, , numeric]) => numeric === key)
  if (!country) return []
  const [alpha2, alpha3] = country
  const aliases = Object.entries(countryAliases)
    .filter(([, target]) => target === alpha3)
    .map(([alias]) => alias)
  return [alpha2, alpha3, ...aliases]
}

export function buildGazetteer(
  name: AtlasName,
  features: readonly Feature[]
): Gazetteer {
  const places = new Map<string, LonLat>()
  for (const feature of features) {
    const place = centroidOf(feature.geometry)
    if (!place) continue
    const title = feature.properties.name
    const keys = [
      ...(typeof title === "string" ? [title] : []),
      ...codesOf(name, feature.id),
    ]
    for (const key of keys) {
      const lookup = normal(key)
      if (!places.has(lookup)) places.set(lookup, place)
    }
  }
  return {
    find: (value) => {
      if (typeof value !== "string" && typeof value !== "number") return null
      return places.get(normal(String(value))) ?? null
    },
  }
}
