import { QueryClient } from "@tanstack/react-query"
import { describe, expect, it } from "vitest"

import { holdOrder, keepOrder } from "./held-order"

const item = (id: string) => ({ id })
const ids = (items: { id: string }[]) => items.map(({ id }) => id)

describe("keepOrder", () => {
  it("returns the fetched order when nothing is held", () => {
    expect(ids(keepOrder([], [item("b"), item("a")]))).toEqual(["b", "a"])
  })

  it("keeps held items where they were and puts new ones first", () => {
    expect(
      ids(keepOrder(["a", "b", "c"], [item("c"), item("d"), item("a")]))
    ).toEqual(["d", "a", "c"])
  })

  it("drops what is no longer fetched", () => {
    expect(ids(keepOrder(["a", "b"], [item("b")]))).toEqual(["b"])
  })
})

describe("holdOrder", () => {
  it("holds one order for each client", () => {
    const client = new QueryClient()
    holdOrder(client, [item("a"), item("b")])
    expect(ids(holdOrder(client, [item("b"), item("a")]))).toEqual(["a", "b"])
    expect(ids(holdOrder(new QueryClient(), [item("b"), item("a")]))).toEqual([
      "b",
      "a",
    ])
  })
})
