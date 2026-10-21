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
  }
}

const schemaPanelTypes = [
  schema.$defs.dataPanel,
  schema.$defs.textPanel,
  schema.$defs.placeholderPanel,
].flatMap((def) => def.properties.type.enum ?? [def.properties.type.const!])

const entries = Object.entries(found).map(([path, { manifest }]) => ({
  folder: path.split("/")[1],
  manifest,
}))

describe("renderer registry", () => {
  it("has one manifest per schema panel type, each named after its folder", () => {
    expect(entries.map(({ manifest }) => manifest.type).sort()).toEqual(
      [...schemaPanelTypes].sort()
    )
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
