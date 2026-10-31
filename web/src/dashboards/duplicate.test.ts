import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import {
  duplicateBlockReason,
  duplicateTargets,
  duplicateTitle,
} from "./duplicate"

function folder(
  id: string,
  title: string,
  create: boolean,
  general = false
): Folder {
  return {
    id,
    title,
    general,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: create, manage: create, create },
    level: create ? "admin" : "comment",
  }
}

const codePoints = (text: string) => [...text].length

describe("duplicateTitle", () => {
  it("appends copy to the title", () => {
    expect(duplicateTitle("Checkout")).toBe("Checkout copy")
  })

  it("trims surrounding spaces", () => {
    expect(duplicateTitle("  Checkout  ")).toBe("Checkout copy")
  })

  it("keeps a 200 code point title within 200 code points", () => {
    const result = duplicateTitle("😀".repeat(200))
    expect(codePoints(result)).toBe(200)
    expect(result.endsWith(" copy")).toBe(true)
    expect(result.startsWith("😀".repeat(195))).toBe(true)
  })

  it("trims the end of a title cut inside a run of spaces", () => {
    const result = duplicateTitle(`${"a".repeat(194)}   b`)
    expect(result).toBe(`${"a".repeat(194)} copy`)
  })
})

describe("duplicateTargets", () => {
  const checkout = folder("c", "Checkout", true)
  const payments = folder("p", "Payments", true)
  const general = folder("g", "General", true, true)

  it("starts in the dashboard's own folder when it can create there", () => {
    expect(duplicateTargets([checkout, payments, general], "p")).toStrictEqual({
      choices: [checkout, payments, general],
      initial: "p",
    })
  })

  it("falls back to General when the own folder is not creatable", () => {
    const locked = folder("l", "Locked", false)
    expect(duplicateTargets([checkout, locked, general], "l")).toStrictEqual({
      choices: [checkout, general],
      initial: "g",
    })
  })

  it("falls back to the first choice when General is not creatable", () => {
    const locked = folder("l", "Locked", false)
    const closedGeneral = folder("g", "General", false, true)
    expect(
      duplicateTargets([locked, payments, checkout, closedGeneral], "l")
    ).toStrictEqual({ choices: [payments, checkout], initial: "p" })
  })

  it("offers nothing when no folder allows creating", () => {
    expect(duplicateTargets([folder("l", "Locked", false)], "l")).toStrictEqual(
      { choices: [], initial: null }
    )
  })
})

describe("duplicateBlockReason", () => {
  it.each([
    [{ pinned: false, editingLayout: false }, null],
    [{ pinned: true, editingLayout: false }, "open the latest version first"],
    [{ pinned: false, editingLayout: true }, "finish the layout first"],
    [{ pinned: true, editingLayout: true }, "open the latest version first"],
  ])("for %j gives %j", (state, expected) => {
    expect(duplicateBlockReason(state)).toBe(expected)
  })
})
