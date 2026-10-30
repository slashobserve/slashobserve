import { feature, mesh } from "topojson-client"

import { parseGeojson, type Feature } from "./layers/geojson"
import { project } from "./projection"

type MeshArguments = Parameters<typeof mesh>
type Topology = MeshArguments[0]
type FeatureArguments = Parameters<typeof feature>

export type AtlasName = "countries" | "usa-states"

const precision = 1e5

const topologies: Partial<Record<AtlasName, Promise<Topology>>> = {}
const features: Partial<Record<AtlasName, Promise<Feature[]>>> = {}
let outline: Promise<string> | undefined

function round(value: number): number {
  return Math.round(value * precision) / precision
}

const antimeridianJump = 180

function linePath(line: readonly (readonly number[])[]): string {
  let path = ""
  let previous: number | null = null
  for (const [lon, lat] of line) {
    const { x, y } = project(lon, lat)
    const breaks =
      previous === null || Math.abs(lon - previous) > antimeridianJump
    path += `${breaks ? "M" : "L"}${round(x)} ${round(y)}`
    previous = lon
  }
  return path
}

async function loadTopology(name: AtlasName): Promise<Topology> {
  const { default: atlas } =
    name === "countries"
      ? await import("world-atlas/countries-110m.json")
      : await import("us-atlas/states-10m.json")
  return atlas as Topology
}

function topology(name: AtlasName): Promise<Topology> {
  topologies[name] ??= loadTopology(name)
  return topologies[name]
}

async function buildOutline(): Promise<string> {
  const atlas = await topology("countries")
  const countries = atlas.objects.countries as MeshArguments[1]
  const borders = mesh(atlas, countries)
  return borders.coordinates.map(linePath).join("")
}

export function worldOutline(): Promise<string> {
  outline ??= buildOutline()
  return outline
}

async function buildFeatures(name: AtlasName): Promise<Feature[]> {
  const atlas = await topology(name)
  const object = atlas.objects[name === "countries" ? "countries" : "states"]
  const collection = feature(atlas, object as FeatureArguments[1])
  return parseGeojson(collection).map((entry) => ({
    ...entry,
    properties: { id: entry.id, name: entry.properties.name },
  }))
}

export function atlasFeatures(name: AtlasName): Promise<Feature[]> {
  features[name] ??= buildFeatures(name)
  return features[name]
}
