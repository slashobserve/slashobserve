import { mesh } from "topojson-client"

import { project } from "./projection"

type MeshArguments = Parameters<typeof mesh>

const precision = 1e5

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

async function buildOutline(): Promise<string> {
  const { default: atlas } = await import("world-atlas/countries-110m.json")
  const topology = atlas as MeshArguments[0]
  const countries = topology.objects.countries as MeshArguments[1]
  const borders = mesh(topology, countries)
  return borders.coordinates.map(linePath).join("")
}

export function worldOutline(): Promise<string> {
  outline ??= buildOutline()
  return outline
}
