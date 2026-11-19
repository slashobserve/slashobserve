import { describe, expect, it } from "vitest"

import { readSpecSource, specSummaryLine } from "./spec-source"

const exported = {
  specVersion: "0",
  title: "Checkout",
  variables: [{ name: "env" }],
  panels: [
    { id: "p1", type: "stat" },
    { id: "r1", type: "row", panels: [{ id: "p2" }, { id: "p3" }] },
  ],
}

describe("readSpecSource", () => {
  it("recognises the Spec the product exports and counts panels inside rows", () => {
    const source = readSpecSource(JSON.stringify(exported, null, 2))
    expect(source?.title).toBe("Checkout")
    expect(source?.panels).toBe(3)
    expect(source?.variables).toBe(1)
    expect(source && specSummaryLine(source)).toBe(
      "ObserveSpec dashboard · 3 panels · 1 variable"
    )
  })

  it("leaves a Grafana dashboard, bad JSON and other values to the Grafana importer", () => {
    expect(
      readSpecSource(
        JSON.stringify({ title: "G", schemaVersion: 39, panels: [] })
      )
    ).toBeNull()
    expect(readSpecSource("not json")).toBeNull()
    expect(readSpecSource("[1, 2]")).toBeNull()
    expect(readSpecSource('{"specVersion": "0"}')).toBeNull()
  })
})
