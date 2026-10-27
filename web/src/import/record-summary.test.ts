import { describe, expect, it } from "vitest"

import { countsText, fixPrompt, importStatus } from "./record-summary"

const none = {
  total: 0,
  imported: 0,
  partial: 0,
  placeholder: 0,
  unverified: 0,
}

describe("countsText", () => {
  it("lists every part", () => {
    expect(
      countsText({
        total: 9,
        imported: 6,
        partial: 1,
        placeholder: 1,
        unverified: 1,
      })
    ).toBe(
      "9 panels at import: 6 imported, 1 partial, 1 placeholder, 1 unverified"
    )
  })

  it("omits parts that are zero", () => {
    expect(countsText({ ...none, total: 4, imported: 3, partial: 1 })).toBe(
      "4 panels at import: 3 imported, 1 partial"
    )
  })

  it("uses the singular for one panel", () => {
    expect(countsText({ ...none, total: 1, imported: 1 })).toBe(
      "1 panel at import: 1 imported"
    )
  })

  it("has no list when every part is zero", () => {
    expect(countsText(none)).toBe("0 panels at import")
  })
})

describe("importStatus", () => {
  it("warns about unverified panels first", () => {
    expect(
      importStatus({ ...none, total: 5, placeholder: 2, unverified: 3 })
    ).toEqual({ tone: "warning", text: "3 unverified" })
  })

  it("counts placeholders without a tone", () => {
    expect(
      importStatus({ ...none, total: 3, imported: 1, placeholder: 2 })
    ).toEqual({ tone: "none", text: "2 placeholders" })
  })

  it("uses the singular for one placeholder", () => {
    expect(
      importStatus({ ...none, total: 3, imported: 2, placeholder: 1 })
    ).toEqual({ tone: "none", text: "1 placeholder" })
  })

  it("is ok when nothing needs a look", () => {
    expect(
      importStatus({ ...none, total: 3, imported: 2, partial: 1 })
    ).toEqual({
      tone: "ok",
      text: "Imported",
    })
  })
})

describe("fixPrompt", () => {
  it("names the panel by title and id", () => {
    expect(fixPrompt('Errors for "checkout"', "p-7")).toBe(
      'Fix the unverified panel "Errors for "checkout"" (p-7).'
    )
  })

  it("falls back to the id when the title is empty", () => {
    expect(fixPrompt("", "p-7")).toBe("Fix the unverified panel p-7.")
  })
})
