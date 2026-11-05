import { describe, expect, it } from "vitest"

import {
  mappingToSend,
  openMatches,
  prefilledMapping,
  unsupported,
  type GrafanaDatasource,
} from "./mapping"

function datasource(
  uid: string,
  kind: string | null,
  candidates: string[],
  matched: string | null
): GrafanaDatasource {
  return {
    uid,
    name: uid,
    type: kind ?? "loki",
    kind,
    candidates,
    matched_uid: matched,
  }
}

const matched = datasource("graf-prom", "prometheus", ["prom", "prom2"], "prom")
const open = datasource("graf-prom-b", "prometheus", ["prom", "prom2"], null)
const noCandidates = datasource("graf-ch", "clickhouse", [], null)
const loki = datasource("graf-loki", null, [], null)
const all = [matched, open, noCandidates, loki]

describe("prefilledMapping", () => {
  it("prefills only supported datasources the server matched", () => {
    expect(prefilledMapping(all)).toEqual({ "graf-prom": "prom" })
  })
})

describe("mappingToSend", () => {
  it.each([
    ["the automatic match is not sent", { "graf-prom": "prom" }, {}],
    [
      "a changed match is sent",
      { "graf-prom": "prom2" },
      { "graf-prom": "prom2" },
    ],
    [
      "a pick for an open match is sent",
      { "graf-prom-b": "prom" },
      { "graf-prom-b": "prom" },
    ],
    ["a pick that is not a candidate is dropped", { "graf-prom-b": "ch" }, {}],
    [
      "a pick for an unsupported datasource is dropped",
      { "graf-loki": "prom" },
      {},
    ],
    ["no picks send nothing", {}, {}],
  ])("%s", (_name, picks, want) => {
    expect(mappingToSend(all, picks)).toEqual(want)
  })
})

describe("openMatches", () => {
  it.each([
    ["no picks", {}, 2],
    ["one open match picked", { "graf-prom-b": "prom" }, 1],
    ["a pick that is not a candidate stays open", { "graf-prom-b": "x" }, 2],
  ])("%s", (_name, picks, want) => {
    expect(openMatches(all, picks)).toBe(want)
  })
})

describe("unsupported", () => {
  it("lists the datasources with no kind", () => {
    expect(unsupported(all)).toEqual([loki])
  })
})
