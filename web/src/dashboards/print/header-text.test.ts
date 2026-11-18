import { describe, expect, it } from "vitest"

import type { VariableState } from "../use-variables"
import { headerLines, stampedAt, variableLines } from "./header-text"

function state(
  name: string,
  selected: string[],
  label?: string
): VariableState {
  return { variable: { name, label }, selected } as VariableState
}

describe("variableLines", () => {
  it("names each variable by its label with its selected values", () => {
    expect(
      variableLines([
        state("env", ["prod"], "Environment"),
        state("region", ["eu", "us"]),
        state("empty", []),
      ])
    ).toEqual(["Environment: prod", "region: eu, us"])
  })
})

describe("headerLines", () => {
  it("joins the folder and version, the range and the variables", () => {
    expect(
      headerLines({
        folder: "Ops",
        version: 7,
        range: "Last 6 hours",
        variables: ["env: prod", "region: eu"],
      })
    ).toEqual({
      context: "Ops, version 7",
      range: "Time range: Last 6 hours",
      variables: "env: prod; region: eu",
    })
  })

  it("leaves out the folder and the variables when there are none", () => {
    expect(
      headerLines({
        folder: null,
        version: 1,
        range: "Last 1 hour",
        variables: [],
      })
    ).toEqual({
      context: "version 1",
      range: "Time range: Last 1 hour",
      variables: null,
    })
  })
})

describe("stampedAt", () => {
  it("writes the moment of printing or saving", () => {
    const at = new Date("2026-11-19T14:05:00Z")
    expect(stampedAt("Printed", at, "en-GB", "UTC")).toMatch(
      /^Printed 19 Nov 2026(,| at) 14:05$/
    )
    expect(stampedAt("Saved", at, "en-GB", "UTC")).toMatch(
      /^Saved 19 Nov 2026(,| at) 14:05$/
    )
  })
})
