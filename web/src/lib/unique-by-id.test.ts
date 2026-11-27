import { describe, expect, it } from "vitest"

import { uniqueById } from "./unique-by-id"

describe("uniqueById", () => {
  it("keeps the first copy of each id across pages, in order", () => {
    const pages = [
      { items: [{ id: "a" }, { id: "b" }] },
      { items: [{ id: "b", late: true }, { id: "c" }] },
    ]
    expect(uniqueById(pages, (page) => page.items)).toEqual([
      { id: "a" },
      { id: "b" },
      { id: "c" },
    ])
  })

  it("returns nothing before the first page", () => {
    expect(uniqueById(undefined, () => [{ id: "a" }])).toEqual([])
  })
})
