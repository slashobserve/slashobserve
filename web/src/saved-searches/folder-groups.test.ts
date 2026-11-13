import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import { groupSavedSearches, savedSearchCountText } from "./folder-groups"
import type { SavedSearch } from "./queries"

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return { id, title, general: false, personal: false, ...extra } as Folder
}

function search(id: string, inFolder: Folder) {
  return { id, folder: { id: inFolder.id } } as SavedSearch
}

const mine = folder("m", "Ana", { personal: true })
const shop = folder("s", "Shop")
const ops = folder("o", "Ops")

function summary(sections: ReturnType<typeof groupSavedSearches>) {
  return sections.map((section) => [
    section.folder.title,
    section.rows.map((row) => row.id),
  ])
}

describe("groupSavedSearches", () => {
  it("lists folders in the folder list's order, so My Dashboards comes first", () => {
    const sections = groupSavedSearches(
      [mine, shop, ops],
      [search("a", ops), search("b", shop), search("c", mine)]
    )
    expect(summary(sections)).toEqual([
      ["Ana", ["c"]],
      ["Shop", ["b"]],
      ["Ops", ["a"]],
    ])
  })

  it("keeps the order the searches arrived in within a folder", () => {
    const sections = groupSavedSearches(
      [shop, ops],
      [search("a", shop), search("b", ops), search("c", shop)]
    )
    expect(summary(sections)).toEqual([
      ["Shop", ["a", "c"]],
      ["Ops", ["b"]],
    ])
  })

  it("leaves out folders without a saved search", () => {
    expect(
      summary(groupSavedSearches([mine, shop, ops], [search("a", shop)]))
    ).toEqual([["Shop", ["a"]]])
  })

  it("returns nothing without searches or without folders", () => {
    expect(groupSavedSearches([mine, shop], [])).toEqual([])
    expect(groupSavedSearches([], [search("a", shop)])).toEqual([])
  })
})

describe("savedSearchCountText", () => {
  it.each([
    [0, "0 saved searches"],
    [1, "1 saved search"],
    [2, "2 saved searches"],
  ])("writes %i as %s", (n, text) => {
    expect(savedSearchCountText(n)).toBe(text)
  })
})
