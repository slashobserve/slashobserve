import { describe, expect, it } from "vitest"

import { rendererManifests } from "./index"
import type { RendererManifest } from "./types"

const found = import.meta.glob<{ manifest: RendererManifest }>(
  "./*/manifest.ts",
  { eager: true }
)

const entries = Object.entries(found).map(([path, { manifest }]) => ({
  folder: path.split("/")[1],
  manifest,
}))

describe("renderer registry", () => {
  it("has a manifest for each of the twelve panel types, named after its folder", () => {
    expect(entries.map((entry) => entry.folder).sort()).toEqual([
      "bargauge",
      "gauge",
      "heatmap",
      "histogram",
      "logs",
      "piechart",
      "placeholder",
      "stat",
      "state-timeline",
      "table",
      "text",
      "timeseries",
    ])
    for (const { folder, manifest } of entries) {
      expect(manifest.type).toBe(folder)
    }
  })

  it("keeps every manifest: no two share a type", () => {
    expect(rendererManifests.size).toBe(entries.length)
  })

  it("supports exactly the types that have a renderer", () => {
    const supported = [...rendererManifests.values()]
      .filter((manifest) => manifest.supported)
      .map((manifest) => manifest.type)
      .sort()
    expect(supported).toEqual(["bargauge", "stat", "table", "timeseries"])
  })
})
