import { describe, expect, it } from "vitest"

import { countText, groupByFolder } from "./group-by-folder"
import type { Folder } from "./queries"

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return { id, title, general: false, personal: false, ...extra } as Folder
}

type Item = { id: string; folderId: string }

function item(id: string, inFolder: Folder): Item {
  return { id, folderId: inFolder.id }
}

const mine = folder("m", "Ana", { personal: true })
const shop = folder("s", "Shop")
const ops = folder("o", "Ops")

function group(folders: Folder[], items: Item[]) {
  return groupByFolder(folders, items, (entry) => entry.folderId).map(
    (section) => [section.folder.title, section.rows.map((row) => row.id)]
  )
}

describe("groupByFolder", () => {
  it("lists folders in the folder list's order, so My Dashboards comes first", () => {
    expect(
      group(
        [mine, shop, ops],
        [item("a", ops), item("b", shop), item("c", mine)]
      )
    ).toEqual([
      ["Ana", ["c"]],
      ["Shop", ["b"]],
      ["Ops", ["a"]],
    ])
  })

  it("keeps the order the items arrived in within a folder", () => {
    expect(
      group([shop, ops], [item("a", shop), item("b", ops), item("c", shop)])
    ).toEqual([
      ["Shop", ["a", "c"]],
      ["Ops", ["b"]],
    ])
  })

  it("leaves out folders without an item", () => {
    expect(group([mine, shop, ops], [item("a", shop)])).toEqual([
      ["Shop", ["a"]],
    ])
  })

  it("returns nothing without items or without folders", () => {
    expect(group([mine, shop], [])).toEqual([])
    expect(group([], [item("a", shop)])).toEqual([])
  })

  it("leaves out items whose folder is not in the list", () => {
    expect(group([shop], [item("a", ops), item("b", shop)])).toEqual([
      ["Shop", ["b"]],
    ])
  })
})

describe("countText", () => {
  it.each([
    [0, "0 saved searches"],
    [1, "1 saved search"],
    [2, "2 saved searches"],
  ])("writes %i as %s", (n, text) => {
    expect(countText(n, "saved search", "saved searches")).toBe(text)
  })

  it("uses the words it is given", () => {
    expect(countText(1, "observation", "observations")).toBe("1 observation")
    expect(countText(3, "observation", "observations")).toBe("3 observations")
  })
})
