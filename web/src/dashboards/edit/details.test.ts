import { describe, expect, it } from "vitest"

import type { Dashboard } from "@/spec"

import {
  applyDetails,
  descriptionError,
  detailsBlockReason,
  detailsOf,
  detailsSummary,
  parseTags,
  sameDetails,
  titleError,
} from "./details"

const base: Dashboard = {
  specVersion: "0",
  title: "Checkout",
  description: "Checkout health",
  tags: ["checkout", "prod"],
  time: { from: "now-6h", to: "now" },
  panels: [
    {
      id: "p1",
      type: "text",
      title: "Notes",
      gridPos: { x: 0, y: 0, w: 12, h: 4 },
      content: "hello",
    },
  ],
} as Dashboard

describe("detailsOf", () => {
  it("reads the three fields", () => {
    expect(detailsOf(base)).toEqual({
      title: "Checkout",
      description: "Checkout health",
      tags: ["checkout", "prod"],
    })
  })

  it("defaults a missing description to empty text and missing tags to none", () => {
    const bare = { ...base }
    delete bare.description
    delete bare.tags
    expect(detailsOf(bare)).toEqual({
      title: "Checkout",
      description: "",
      tags: [],
    })
  })
})

describe("parseTags", () => {
  it("splits on commas and whitespace, lowercases and de-duplicates in first-seen order", () => {
    expect(parseTags("API, api  web")).toEqual({ tags: ["api", "web"] })
  })

  it("lowercases an uppercase tag instead of rejecting it", () => {
    expect(parseTags("Prod")).toEqual({ tags: ["prod"] })
  })

  it("accepts an empty input as no tags", () => {
    expect(parseTags(" , ,")).toEqual({ tags: [] })
  })

  it("names every tag that does not match the pattern", () => {
    const result = parseTags("ok -x y!z")
    expect(result).toEqual({
      error:
        "Tags use letters, digits, dots, dashes and underscores, start with a letter or digit, and have at most 40 characters: -x, y!z",
    })
  })

  it("limits a tag to 40 characters", () => {
    expect(parseTags("a".repeat(40))).toEqual({ tags: ["a".repeat(40)] })
    expect("error" in parseTags("a".repeat(41))).toBe(true)
  })

  it("allows 20 tags and rejects 21", () => {
    const tags = Array.from({ length: 21 }, (_, index) => `t${index}`)
    expect(parseTags(tags.slice(0, 20).join(" "))).toEqual({
      tags: tags.slice(0, 20),
    })
    expect(parseTags(tags.join(" "))).toEqual({
      error: "Use at most 20 tags.",
    })
  })
})

describe("titleError and descriptionError", () => {
  it("requires a title after trimming", () => {
    expect(titleError("   ")).toBe("Enter a title.")
    expect(titleError(" Checkout ")).toBeNull()
  })

  it("counts title characters by code point", () => {
    expect(titleError("a".repeat(200))).toBeNull()
    expect(titleError("a".repeat(201))).toBe("Use at most 200 characters.")
    expect(titleError("😀".repeat(200))).toBeNull()
    expect(titleError("😀".repeat(201))).toBe("Use at most 200 characters.")
  })

  it("limits the description to 2000 characters by code point", () => {
    expect(descriptionError("")).toBeNull()
    expect(descriptionError("😀".repeat(2000))).toBeNull()
    expect(descriptionError("a".repeat(2001))).toBe(
      "Use at most 2000 characters."
    )
  })
})

describe("applyDetails", () => {
  it("trims the title and sets the description and tags", () => {
    const next = applyDetails(base, {
      title: "  Orders  ",
      description: " Order flow ",
      tags: ["orders"],
    })
    expect(next.title).toBe("Orders")
    expect(next.description).toBe("Order flow")
    expect(next.tags).toEqual(["orders"])
  })

  it("removes a blank description and empty tags and leaves the other keys alone", () => {
    const next = applyDetails(base, {
      title: "Checkout",
      description: "   ",
      tags: [],
    })
    expect("description" in next).toBe(false)
    expect("tags" in next).toBe(false)
    expect(next.panels).toEqual(base.panels)
    expect(next.time).toEqual(base.time)
  })

  it("does not change the input", () => {
    const before = structuredClone(base)
    applyDetails(base, { title: "x", description: "", tags: [] })
    expect(base).toEqual(before)
  })
})

describe("sameDetails", () => {
  const a = { title: "A", description: "d", tags: ["x", "y"] }

  it("is true for equal values", () => {
    expect(sameDetails(a, { ...a, tags: ["x", "y"] })).toBe(true)
  })

  it("is false when any field differs, tag order included", () => {
    expect(sameDetails(a, { ...a, title: "B" })).toBe(false)
    expect(sameDetails(a, { ...a, description: "" })).toBe(false)
    expect(sameDetails(a, { ...a, tags: ["y", "x"] })).toBe(false)
    expect(sameDetails(a, { ...a, tags: ["x"] })).toBe(false)
  })
})

describe("detailsSummary", () => {
  const before = { title: "Checkout", description: "d", tags: ["a"] }

  it("names the new title when only the title changed", () => {
    expect(detailsSummary(before, { ...before, title: "Orders" })).toBe(
      'Rename to "Orders"'
    )
  })

  it("uses the general summary for any other change", () => {
    expect(detailsSummary(before, { ...before, description: "e" })).toBe(
      "Edit dashboard details"
    )
    expect(
      detailsSummary(before, { ...before, title: "Orders", tags: ["b"] })
    ).toBe("Edit dashboard details")
  })
})

describe("detailsBlockReason", () => {
  it("gives the first matching reason in order of precedence", () => {
    expect(
      detailsBlockReason({
        pinned: true,
        editingLayout: true,
        editingPanel: true,
      })
    ).toBe("open the latest version first")
    expect(
      detailsBlockReason({
        pinned: false,
        editingLayout: true,
        editingPanel: true,
      })
    ).toBe("finish the layout first")
    expect(
      detailsBlockReason({
        pinned: false,
        editingLayout: false,
        editingPanel: true,
      })
    ).toBe("close the panel editor first")
    expect(
      detailsBlockReason({
        pinned: false,
        editingLayout: false,
        editingPanel: false,
      })
    ).toBeNull()
  })
})
