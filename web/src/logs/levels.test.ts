import { describe, expect, it } from "vitest"

import { hiddenLevels, levelsParam, parseLevels, pickLevel } from "./levels"
import { validateLogsSearch } from "./search"

describe("parseLevels", () => {
  it("reads the shown levels in chip order", () => {
    expect(parseLevels("info,error")).toEqual(["error", "info"])
  })

  it("drops unknown names and repeats", () => {
    expect(parseLevels("error,nope,error")).toEqual(["error"])
  })

  it.each([undefined, "", "nope", "error,warning,info,other", 3])(
    "means all levels for %j",
    (value) => {
      expect(parseLevels(value)).toBeUndefined()
    }
  )

  it("reads the first of repeated parameters", () => {
    expect(parseLevels(["warning", "error"])).toEqual(["warning"])
  })

  it("is part of the logs search", () => {
    expect(validateLogsSearch({ levels: "warning,error" }).levels).toBe(
      "error,warning"
    )
    expect(validateLogsSearch({ levels: "nope" }).levels).toBeUndefined()
  })
})

describe("levelsParam", () => {
  it("writes the shown levels, and nothing for all", () => {
    expect(levelsParam(["error", "info"])).toBe("error,info")
    expect(levelsParam(undefined)).toBeUndefined()
  })
})

describe("hiddenLevels", () => {
  it("hides every level that is not shown", () => {
    expect([...hiddenLevels(["error"])]).toEqual(["warning", "info", "other"])
  })

  it("hides nothing when all are shown", () => {
    expect(hiddenLevels(undefined).size).toBe(0)
  })
})

describe("pickLevel", () => {
  it("shows only the clicked level", () => {
    expect(pickLevel(undefined, "error", false)).toEqual(["error"])
    expect(pickLevel(["info", "warning"], "error", false)).toEqual(["error"])
  })

  it("clears when the only shown level is clicked again", () => {
    expect(pickLevel(["error"], "error", false)).toBeUndefined()
  })

  it("narrows a set to one level when a member is clicked plainly", () => {
    expect(pickLevel(["error", "info"], "error", false)).toEqual(["error"])
  })

  it("adds a level to the set with a modifier", () => {
    expect(pickLevel(["error"], "info", true)).toEqual(["error", "info"])
  })

  it("removes a level from the set with a modifier", () => {
    expect(pickLevel(["error", "info"], "error", true)).toEqual(["info"])
  })

  it("hides one level out of all with a modifier", () => {
    expect(pickLevel(undefined, "info", true)).toEqual([
      "error",
      "warning",
      "other",
    ])
  })

  it("returns to all when the set is emptied or completed", () => {
    expect(pickLevel(["error"], "error", true)).toBeUndefined()
    expect(
      pickLevel(["error", "warning", "info"], "other", true)
    ).toBeUndefined()
  })
})
