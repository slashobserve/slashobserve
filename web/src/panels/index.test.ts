import { describe, expect, it } from "vitest"

import schemaText from "../../../spec/schema/observespec.v0.json?raw"

import { rendererManifests } from "./index"
import type { RendererManifest } from "./types"

const found = import.meta.glob<{ manifest: RendererManifest }>(
  "./*/manifest.ts",
  { eager: true }
)

type PanelDef = { properties: { type: { enum?: string[]; const?: string } } }

const schema = JSON.parse(schemaText) as {
  $defs: {
    dataPanel: PanelDef
    textPanel: PanelDef
    placeholderPanel: PanelDef
    dashlistPanel: PanelDef
  }
}

const schemaPanelTypes = [
  schema.$defs.dataPanel,
  schema.$defs.textPanel,
  schema.$defs.placeholderPanel,
  schema.$defs.dashlistPanel,
].flatMap((def) => def.properties.type.enum ?? [def.properties.type.const!])

const entries = Object.entries(found).map(([path, { manifest }]) => ({
  folder: path.split("/")[1],
  manifest,
}))

describe("renderer registry", () => {
  it("has only manifests of schema panel types, each named after its folder", () => {
    for (const { manifest } of entries) {
      expect(schemaPanelTypes).toContain(manifest.type)
    }
    for (const { folder, manifest } of entries) {
      expect(manifest.type).toBe(folder)
    }
  })

  it("has a manifest for every schema panel type", () => {
    const manifested = entries.map(({ manifest }) => manifest.type)
    for (const type of schemaPanelTypes) {
      expect(manifested).toContain(type)
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
