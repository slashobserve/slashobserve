import { describe, expect, it } from "vitest"

import {
  datasourceRefType,
  datasourceVariableDefaults,
  datasourceVariableName,
  resolveDatasourceRef,
} from "./datasource-ref"
import type { Variable } from "./observespec.gen"

const variables: Variable[] = [
  {
    name: "ds",
    type: "datasource",
    datasourceType: "prometheus",
    default: "prom",
  },
  { name: "logs", type: "datasource", datasourceType: "clickhouse" },
  { name: "job", type: "custom", options: ["a", "b"] },
]

describe("datasourceVariableName", () => {
  it.each([
    ["${ds}", "ds"],
    ["${a_1}", "a_1"],
    ["${" + "a".repeat(40) + "}", "a".repeat(40)],
    ["prom", undefined],
    ["", undefined],
    ["$ds", undefined],
    ["${ds:text}", undefined],
    ["[[ds]]", undefined],
    ["${}", undefined],
    ["${1ds}", undefined],
    ["${_ds}", undefined],
    ["${" + "a".repeat(41) + "}", undefined],
    [" ${ds}", undefined],
    ["${ds} ", undefined],
    ["${ds}${ds}", undefined],
  ])("reads %j as %j", (ref, name) => {
    expect(datasourceVariableName(ref)).toBe(name)
  })
})

describe("resolveDatasourceRef", () => {
  it.each([
    ["a plain uid", "prom", {}, "prom"],
    ["a plain uid ignores selections", "prom", { prom: ["other"] }, "prom"],
    [
      "a reference with a selection",
      "${ds}",
      { ds: ["prom_tight"] },
      "prom_tight",
    ],
    ["a reference without a selection", "${ds}", {}, undefined],
    ["a reference with an empty selection", "${ds}", { ds: [] }, undefined],
    ["a reference with two values", "${ds}", { ds: ["a", "b"] }, undefined],
    ["a reference selecting All", "${ds}", { ds: ["$__all"] }, undefined],
    ["a reference named like a prototype key", "${constructor}", {}, undefined],
  ])("%s", (_name, ref, selections, expected) => {
    expect(resolveDatasourceRef(ref, selections)).toBe(expected)
  })
})

describe("datasourceRefType", () => {
  const typeOfUid = (uid: string) => (uid === "prom" ? "prometheus" : undefined)

  it.each([
    ["a uid", "prom", "prometheus"],
    ["an unknown uid", "nope", undefined],
    ["a reference to a datasource variable", "${ds}", "prometheus"],
    ["a reference to another datasource variable", "${logs}", "clickhouse"],
    ["a reference to a custom variable", "${job}", undefined],
    ["a reference to a missing variable", "${missing}", undefined],
  ])("%s", (_name, ref, expected) => {
    expect(datasourceRefType(ref, variables, typeOfUid)).toBe(expected)
  })
})

describe("datasourceVariableDefaults", () => {
  it("keeps only datasource variables that have a default", () => {
    expect(datasourceVariableDefaults(variables)).toEqual({ ds: ["prom"] })
  })

  it("is empty without variables", () => {
    expect(datasourceVariableDefaults([])).toEqual({})
  })
})
