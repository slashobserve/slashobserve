import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { describe, expect, test } from "vitest"

import { parseSearch, stringifySearch } from "@/lib/search"

import { validateHistorySearch } from "./search"

function routeSearch(url: string) {
  const root = createRootRoute()
  const versions = createRoute({
    getParentRoute: () => root,
    path: "/versions",
    validateSearch: validateHistorySearch,
  })
  const router = createRouter({
    routeTree: root.addChildren([versions]),
    history: createMemoryHistory({ initialEntries: [url] }),
    parseSearch,
    stringifySearch,
  })
  return router.matchRoutes(router.state.location).at(-1)?.search
}

describe("validateHistorySearch", () => {
  test.each([
    ["?from=3&to=4", { from: 3, to: 4 }],
    ["?from=0&to=12", { from: 0, to: 12 }],
    ["?from=3&from=9&to=4", { from: 3, to: 4 }],
  ])("keeps valid version numbers in %s", (query, expected) => {
    expect(routeSearch(`/versions${query}`)).toMatchObject(expected)
  })

  test.each([
    "?to=abc",
    "?to=-1",
    "?to=1.5",
    "?to=1e3",
    "?to=0123",
    "?to=1234567890",
    "?to=",
  ])("drops an invalid to in %s from the route search", (query) => {
    const search = routeSearch(`/versions${query}`)
    expect(search?.to).toBeUndefined()
  })

  test("drops an invalid from while keeping a valid to", () => {
    const search = routeSearch("/versions?from=abc&to=4")
    expect(search?.from).toBeUndefined()
    expect(search?.to).toBe(4)
  })

  test("drops a back value that is not a dashboard search", () => {
    const search = routeSearch("/versions?back=%00")
    expect(search?.back).toBeUndefined()
  })
})
