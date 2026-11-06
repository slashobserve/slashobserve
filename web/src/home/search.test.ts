import { describe, expect, test } from "vitest"

import { isEmptySearch, parseHomeQuery, rankResults } from "./search"

describe("parseHomeQuery", () => {
  test.each([
    ["", { tag: [] }],
    ["   ", { tag: [] }],
    ["latency", { q: "latency", tag: [] }],
    ["tag:prod latency", { q: "latency", tag: ["prod"] }],
    ["checkout  tag:prod   p99 ", { q: "checkout p99", tag: ["prod"] }],
    ["tag:prod tag:team-a", { tag: ["prod", "team-a"] }],
    ["tag:prod tag:prod", { tag: ["prod"] }],
    ["TAG:Prod", { tag: ["prod"] }],
    ["tag:", { q: "tag:", tag: [] }],
    ["tag:-bad latency", { q: "tag:-bad latency", tag: [] }],
    ["tag:a/b", { q: "tag:a/b", tag: [] }],
  ])("%j", (text, want) => {
    expect(parseHomeQuery(text)).toEqual(want)
  })

  test("keeps at most ten tags and the rest of the words within 200 characters", () => {
    const tags = Array.from({ length: 12 }, (_, i) => `tag:t${i}`).join(" ")
    expect(parseHomeQuery(tags).tag).toHaveLength(10)
    expect(parseHomeQuery("x".repeat(250)).q).toHaveLength(200)
  })
})

describe("isEmptySearch", () => {
  test("is empty only without text and tags", () => {
    expect(isEmptySearch(parseHomeQuery(" "))).toBe(true)
    expect(isEmptySearch(parseHomeQuery("tag:prod"))).toBe(false)
    expect(isEmptySearch(parseHomeQuery("cpu"))).toBe(false)
  })
})

describe("rankResults", () => {
  const results = [
    { title: "Node exporter" },
    { title: "Shop latency" },
    { title: "Latency by pod" },
    { title: "Payments" },
    { title: "Shop latency-budget" },
    { title: "Checkout slowlatency" },
  ]

  test("puts title prefixes first, then word starts, then any title match, then the rest", () => {
    expect(rankResults(results, "Latency").map((r) => r.title)).toEqual([
      "Latency by pod",
      "Shop latency",
      "Shop latency-budget",
      "Checkout slowlatency",
      "Node exporter",
      "Payments",
    ])
  })

  test("keeps the server order without text", () => {
    expect(rankResults(results, undefined)).toEqual(results)
  })
})
