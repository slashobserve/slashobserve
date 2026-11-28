import type { Transform } from "@/spec"

import type { Gazetteers } from "../geomap/gazetteer/lookup"
import {
  distanceMeters,
  headingDegrees,
  polygonAreaSquareMeters,
  type GeoPoint,
} from "./geodesy"
import { locate, withLocationFields, type Locations } from "./location"
import { copyFrame, type PreparedField, type PreparedFrame } from "./names"

type SpatialOptions = Extract<Transform, { type: "spatial" }>
type Calc = NonNullable<SpatialOptions["calc"]>

const defaultCalc: Calc = "heading"

const calculations: Record<
  Calc,
  { name: string; unit: "degree" | "lengthm" | "areaM2" }
> = {
  heading: { name: "Heading", unit: "degree" },
  distance: { name: "Distance", unit: "lengthm" },
  area: { name: "Area", unit: "areaM2" },
}

function pointAt(places: Locations, row: number): GeoPoint | null {
  const lat = places.lat[row]
  const lon = places.lon[row]
  return lat === null || lat === undefined || lon === null || lon === undefined
    ? null
    : { lat, lon }
}

function points(places: Locations): (GeoPoint | null)[] {
  return places.lat.map((_, row) => pointAt(places, row))
}

function fromPrevious(
  rows: readonly (GeoPoint | null)[],
  measure: (from: GeoPoint, to: GeoPoint) => number
): (number | null)[] {
  let previous: GeoPoint | null = null
  return rows.map((point) => {
    const value = previous && point ? measure(previous, point) : null
    if (point) previous = point
    return value
  })
}

function outlinedArea(rows: readonly (GeoPoint | null)[]): (number | null)[] {
  const corners = rows.filter((point) => point !== null)
  const area = corners.length < 3 ? null : polygonAreaSquareMeters(corners)
  return rows.map(() => area)
}

function calculated(
  frame: PreparedFrame,
  { calc = defaultCalc, field }: SpatialOptions,
  places: Locations
): PreparedFrame {
  const rows = points(places)
  const values =
    calc === "area"
      ? outlinedArea(rows)
      : fromPrevious(rows, calc === "heading" ? headingDegrees : distanceMeters)
  const { name, unit } = calculations[calc]
  const added: PreparedField = {
    name: field ?? name,
    type: "number",
    values,
    config: { unit },
  }
  return { ...frame, fields: [...frame.fields, added] }
}

function pickRows(frame: PreparedFrame, rows: readonly number[]) {
  return {
    ...frame,
    fields: frame.fields.map((field) => ({
      ...field,
      values: rows.map((row) => field.values[row] ?? null),
    })),
  }
}

function located(places: Locations): (GeoPoint & { row: number })[] {
  return places.lat.flatMap((_, row) => {
    const point = pointAt(places, row)
    return point ? [{ ...point, row }] : []
  })
}

function asLine(frame: PreparedFrame, places: Locations): PreparedFrame {
  const found = located(places)
  return withLocationFields(
    pickRows(
      frame,
      found.map(({ row }) => row)
    ),
    {
      lat: found.map(({ lat }) => lat),
      lon: found.map(({ lon }) => lon),
    }
  )
}

function lineBuilder(
  frame: PreparedFrame,
  from: Locations,
  to: Locations
): PreparedFrame[] {
  return located(from).flatMap((start) => {
    const end = pointAt(to, start.row)
    if (!end) return []
    return [
      withLocationFields(pickRows(frame, [start.row, start.row]), {
        lat: [start.lat, end.lat],
        lon: [start.lon, end.lon],
      }),
    ]
  })
}

function modified(
  frame: PreparedFrame,
  options: SpatialOptions,
  places: Locations,
  gazetteers: Gazetteers
): PreparedFrame[] {
  if (options.modify === "asLine") return [asLine(frame, places)]
  const target = locate(frame, options.target, gazetteers)
  return target ? lineBuilder(frame, places, target) : [copyFrame(frame)]
}

export function spatial(
  frames: PreparedFrame[],
  options: SpatialOptions,
  gazetteers: Gazetteers
): PreparedFrame[] {
  return frames.flatMap((frame) => {
    const places = locate(frame, options.source, gazetteers)
    if (!places) return [copyFrame(frame)]
    switch (options.action) {
      case "calculate":
        return [calculated(frame, options, places)]
      case "modify":
        return modified(frame, options, places, gazetteers)
      default:
        return [withLocationFields(frame, places)]
    }
  })
}
