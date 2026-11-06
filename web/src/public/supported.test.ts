import { describe, expect, it } from "vitest"

import type { Variable } from "@/spec"

import {
  availableOnPublicLink,
  viewerMayChoose,
  viewerVars,
  type PublicMode,
} from "./supported"

const job: Variable = {
  name: "job",
  type: "query",
  datasource: "prom",
  query: "",
}
const env: Variable = { name: "env", type: "custom", options: ["prod", "dev"] }
const note: Variable = { name: "note", type: "textbox", default: "hello" }

describe("availableOnPublicLink", () => {
  it.each([
    ["dashlist", false],
    ["news", false],
    ["geomap", false],
    ["timeseries", true],
    ["stat", true],
    ["text", true],
  ])("%s is available: %s", (type, expected) => {
    expect(availableOnPublicLink(type)).toBe(expected)
  })
})

describe("viewerMayChoose", () => {
  it.each<[PublicMode, Variable["type"], boolean]>([
    ["free", "query", true],
    ["free", "custom", true],
    ["free", "interval", true],
    ["free", "datasource", true],
    ["free", "textbox", false],
    ["free", "adhoc", false],
    ["free", "groupby", false],
    ["free", "constant", false],
    ["frozen", "query", false],
    ["frozen", "custom", false],
  ])("%s link, %s variable: %s", (mode, type, expected) => {
    expect(viewerMayChoose(mode, { name: "v", type } as Variable)).toBe(
      expected
    )
  })
})

describe("viewerVars", () => {
  const stored = { job: ["api"], note: ["fixed"] }
  const requested = { job: ["web"], env: ["dev"], note: ["changed"] }

  it("keeps the stored selections on a frozen link", () => {
    expect(viewerVars("frozen", [job, env, note], stored, requested)).toEqual(
      stored
    )
  })

  it("takes the viewer's choice only for choosable variables on a free link", () => {
    expect(viewerVars("free", [job, env, note], stored, requested)).toEqual({
      job: ["web"],
      env: ["dev"],
      note: ["fixed"],
    })
  })

  it("ignores requested names the dashboard does not declare", () => {
    expect(viewerVars("free", [job], {}, { other: ["x"] })).toEqual({})
  })
})
