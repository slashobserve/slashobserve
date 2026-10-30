import type { Transform } from "@/spec"

import {
  atlasOf,
  gazetteerName,
  resolveLookup,
  type Gazetteers,
} from "../geomap/gazetteer/lookup"
import { decodeGeohash } from "../geomap/geohash"
import type { AtlasName } from "../geomap/world"
import { isNumber } from "../reducers"
import {
  matchesName,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type Location = NonNullable<
  Extract<Transform, { type: "spatial" }>["source"]
>

export type Locations = { lat: (number | null)[]; lon: (number | null)[] }

const latitudeNames = ["lat", "latitude"]
const longitudeNames = ["lon", "lng", "longitude"]
const geohashNames = ["geohash"]

const latitudeField = latitudeNames[0]
const longitudeField = longitudeNames[0]

function named(
  frame: PreparedFrame,
  names: readonly string[]
): PreparedField | undefined {
  for (const name of names) {
    const found = frame.fields.find(
      (field) => field.name.toLowerCase() === name
    )
    if (found) return found
  }
  return undefined
}

function pick(
  frame: PreparedFrame,
  wanted: string | undefined,
  fallback: () => PreparedField | undefined
): PreparedField | undefined {
  if (wanted === undefined) return fallback()
  return frame.fields.find((field) => matchesName(frame, field, wanted))
}

function coordinate(value: unknown, limit: number): number | null {
  const number =
    typeof value === "string" && value.trim() !== "" ? Number(value) : value
  return isNumber(number) && Math.abs(number) <= limit ? number : null
}

function fromPlaces(
  frame: PreparedFrame,
  place: (row: number) => { lat: unknown; lon: unknown } | null
): Locations {
  const lat: (number | null)[] = []
  const lon: (number | null)[] = []
  for (let row = 0; row < rowCount(frame); row++) {
    const found = place(row)
    const north = found ? coordinate(found.lat, 90) : null
    const east = found ? coordinate(found.lon, 180) : null
    const both = north !== null && east !== null
    lat.push(both ? north : null)
    lon.push(both ? east : null)
  }
  return { lat, lon }
}

function byCoordinates(
  frame: PreparedFrame,
  location: Location
): Locations | null {
  const north = pick(frame, location.latitude, () =>
    named(frame, latitudeNames)
  )
  const east = pick(frame, location.longitude, () =>
    named(frame, longitudeNames)
  )
  if (!north || !east) return null
  return fromPlaces(frame, (row) => ({
    lat: north.values[row],
    lon: east.values[row],
  }))
}

function byGeohash(frame: PreparedFrame, location: Location): Locations | null {
  const field = pick(frame, location.geohash, () => named(frame, geohashNames))
  if (!field) return null
  return fromPlaces(frame, (row) => {
    const value = field.values[row]
    return typeof value === "string" ? decodeGeohash(value) : null
  })
}

function byLookup(
  frame: PreparedFrame,
  location: Location,
  gazetteers: Gazetteers
): Locations | null {
  const field = pick(frame, location.lookup, () =>
    frame.fields.find((candidate) => candidate.type === "string")
  )
  const state = resolveLookup(gazetteerName(location.gazetteer), gazetteers)
  if (!field || state.state !== "ready") return null
  return fromPlaces(frame, (row) => state.gazetteer.find(field.values[row]))
}

export function locate(
  frame: PreparedFrame,
  location: Location | undefined,
  gazetteers: Gazetteers
): Locations | null {
  const where = location ?? {}
  switch (where.mode) {
    case "coords":
      return byCoordinates(frame, where)
    case "geohash":
      return byGeohash(frame, where)
    case "lookup":
      return byLookup(frame, where, gazetteers)
    default:
      return byCoordinates(frame, {}) ?? byGeohash(frame, {})
  }
}

export function withLocationFields(
  frame: PreparedFrame,
  { lat, lon }: Locations
): PreparedFrame {
  const added: PreparedField[] = [
    { name: latitudeField, type: "number", values: lat },
    { name: longitudeField, type: "number", values: lon },
  ]
  const replaced = frame.fields.map(
    (field) => added.find((next) => next.name === field.name) ?? field
  )
  const fresh = added.filter((next) => !replaced.includes(next))
  return { ...frame, fields: [...replaced, ...fresh] }
}

export function gazetteersNeeded(
  transforms: readonly Transform[] | undefined
): AtlasName[] {
  const names = new Set<AtlasName>()
  const need = (location: Location | undefined) => {
    if (location?.mode !== "lookup") return
    const atlas = atlasOf(gazetteerName(location.gazetteer))
    if (atlas) names.add(atlas)
  }
  for (const transform of transforms ?? []) {
    if (transform.type === "spatial") {
      need(transform.source)
      need(transform.target)
    }
    if (transform.type === "fieldLookup") {
      const atlas = atlasOf(gazetteerName(transform.gazetteer))
      if (atlas) names.add(atlas)
    }
  }
  return [...names]
}
