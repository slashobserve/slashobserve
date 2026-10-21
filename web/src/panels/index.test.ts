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
  it("has twelve manifests, each named after its folder", () => {
    expect(entries).toHaveLength(12)
    for (const { folder, manifest } of entries) {
      expect(manifest.type).toBe(folder)
    }
  })

  it("keeps every manifest: no two share a type", () => {
    expect(rendererManifests.size).toBe(entries.length)
  })

  it("gives every supported manifest a loader", () => {
    for (const manifest of rendererManifests.values()) {
      if (manifest.supported) expect(manifest.load).toBeTypeOf("function")
    }
  })
})
