import { describe, expect, test } from "vitest"

import { paletteGroups, type SearchResult } from "./palette-items"

const thirty = Array.from({ length: 30 }, (_, index) => ({
  id: `d${index}`,
  title: `Dashboard ${index}`,
}))

describe("paletteGroups", () => {
  test("an empty query lists every page, the first 8 dashboards and the account actions", () => {
    const groups = paletteGroups("", thirty, [], "admin")
    expect(groups.map((group) => group.value)).toEqual([
      "Pages",
      "Dashboards",
      "Actions",
      "Account",
    ])
    expect(groups[0]?.items.map((entry) => entry.label)).toEqual([
      "Inbox",
      "Observations",
      "Dashboards",
      "Health",
      "Logs",
      "Traces",
      "Agents",
      "Datasources",
      "API keys",
      "People",
      "Settings",
    ])
    expect(groups[1]?.items.map((entry) => entry.id)).toEqual(
      thirty.slice(0, 8).map((dashboard) => dashboard.id)
    )
    expect(groups[2]?.items.map((entry) => entry.label)).toEqual([
      "New observation",
      "Ask Observe Agent",
      "Import a dashboard",
    ])
    expect(groups[3]?.items.map((entry) => entry.label)).toEqual([
      "Switch theme",
      "Sign out",
    ])
  })

  test("a query matching one page omits the empty groups", () => {
    const groups = paletteGroups("api", thirty, [], "admin")
    expect(groups).toHaveLength(1)
    expect(groups[0]?.value).toBe("Pages")
    expect(groups[0]?.items.map((entry) => entry.label)).toEqual(["API keys"])
    expect(groups[0]?.items[0]).toMatchObject({ kind: "page", to: "/keys" })
  })

  test("matches titles case-insensitively", () => {
    const groups = paletteGroups(
      "CHECK",
      [
        { id: "a", title: "Checkout health" },
        { id: "b", title: "Billing" },
      ],
      [],
      "admin"
    )
    expect(groups).toHaveLength(1)
    expect(groups[0]?.value).toBe("Dashboards")
    expect(groups[0]?.items.map((entry) => entry.id)).toEqual(["a"])
  })

  test("cuts matching dashboards to 20", () => {
    const many = Array.from({ length: 25 }, (_, index) => ({
      id: `m${index}`,
      title: `Match ${index}`,
    }))
    const groups = paletteGroups("match", many, [], "admin")
    expect(groups[0]?.value).toBe("Dashboards")
    expect(groups[0]?.items).toHaveLength(20)
  })

  test("finds sign out in the Account group", () => {
    const groups = paletteGroups("sign", [], [], "admin")
    expect(groups.map((group) => group.value)).toEqual(["Account"])
    expect(groups[0]?.items).toEqual([
      { kind: "action", id: "action:sign-out", label: "Sign out" },
    ])
  })

  test("finds switch theme in the Account group", () => {
    const groups = paletteGroups("THEME", [], [], "viewer")
    expect(groups.map((group) => group.value)).toEqual(["Account"])
    expect(groups[0]?.items).toEqual([
      { kind: "action", id: "action:theme", label: "Switch theme" },
    ])
  })

  test("a Viewer is offered no Admin-only pages", () => {
    const labels = paletteGroups("", [], [], "viewer")[0]?.items.map(
      (entry) => entry.label
    )
    expect(labels).toEqual([
      "Inbox",
      "Observations",
      "Dashboards",
      "Health",
      "Logs",
      "Traces",
      "Agents",
      "Settings",
    ])
    expect(paletteGroups("people", [], [], "viewer")).toEqual([])
    expect(paletteGroups("api keys", [], [], "editor")).toEqual([])
  })

  test("a query finds the New actions by their words", () => {
    const groups = paletteGroups("import", [], [], "viewer")
    expect(groups.map((group) => group.value)).toEqual(["Actions"])
    expect(groups[0]?.items).toEqual([
      { kind: "action", id: "action:import", label: "Import a dashboard" },
    ])
    const ask = paletteGroups("ask", [], [], "viewer")
    expect(ask[0]?.items.map((entry) => entry.id)).toEqual(["action:ask"])
    const agent = paletteGroups("agent", [], [], "viewer")
    expect(agent.map((group) => group.value)).toEqual(["Pages", "Actions"])
    expect(agent[0]?.items.map((entry) => entry.id)).toEqual(["page:/agents"])
    expect(agent[1]?.items.map((entry) => entry.id)).toEqual(["action:ask"])
  })

  test("a query matching nothing returns no groups", () => {
    expect(paletteGroups("zzzz-no-match", thirty, [], "admin")).toEqual([])
  })
})

function result(
  kind: SearchResult["kind"],
  id: string,
  title: string,
  overrides: Partial<SearchResult> = {}
): SearchResult {
  return {
    kind,
    id,
    title,
    snippet: [{ text: `about ${title}`, match: false }],
    url: `https://observe.example.com/d/${id}/${title.toLowerCase()}`,
    folder: { id: "f1", title: "Payments" },
    target: null,
    thread_id: null,
    updated_at: "2026-01-02T03:04:05Z",
    ...overrides,
  }
}

const everyKind: SearchResult[] = [
  result("saved_search", "s1", "Checkout errors"),
  result("annotation", "n1", "Deploy"),
  result("comment", "c1", "Checkout latency", {
    thread_id: "t1",
    target: { kind: "dashboard", id: "d9", title: "Checkout latency" },
  }),
  result("observation", "o1", "Slow pod", {
    url: "https://observe.example.com/o/stale-address",
  }),
  result("dashboard", "d1", "Checkout latency"),
]

describe("paletteGroups with search results", () => {
  test("below two characters the results are ignored", () => {
    const groups = paletteGroups("d", thirty, everyKind, "admin")
    expect(groups.map((group) => group.value)).toEqual([
      "Pages",
      "Dashboards",
      "Actions",
    ])
    expect(groups[1]?.items.map((entry) => entry.id)).toEqual(
      thirty.slice(0, 20).map((dashboard) => dashboard.id)
    )
    expect(paletteGroups(" d ", [], everyKind, "admin")).toEqual(
      paletteGroups("d", [], [], "admin")
    )
  })

  test("from two characters the groups follow Dashboards, Observations, Comments, Annotations, Saved searches", () => {
    const groups = paletteGroups("ch", [], everyKind, "admin")
    expect(groups.map((group) => group.value)).toEqual([
      "Dashboards",
      "Observations",
      "Comments",
      "Annotations",
      "Saved searches",
      "Account",
    ])
  })

  test("local dashboards come first and a server dashboard already listed is not repeated", () => {
    const groups = paletteGroups(
      "checkout",
      [
        { id: "d1", title: "Checkout latency" },
        { id: "d2", title: "Checkout health" },
      ],
      [
        result("dashboard", "d3", "Checkout errors"),
        result("dashboard", "d1", "Checkout latency"),
      ],
      "admin"
    )
    expect(groups[0]?.value).toBe("Dashboards")
    expect(groups[0]?.items.map((entry) => entry.id)).toEqual([
      "d1",
      "d2",
      "d3",
    ])
  })

  test("dashboards are cut to 20 with local ones kept", () => {
    const many = Array.from({ length: 19 }, (_, index) => ({
      id: `m${index}`,
      title: `Match ${index}`,
    }))
    const groups = paletteGroups(
      "match",
      many,
      [
        result("dashboard", "x1", "Match server one"),
        result("dashboard", "x2", "Match server two"),
      ],
      "admin"
    )
    const ids = groups[0]?.items.map((entry) => entry.id)
    expect(ids).toHaveLength(20)
    expect(ids?.at(-1)).toBe("x1")
  })

  test("a result with no page is left out", () => {
    const groups = paletteGroups(
      "deploy",
      [],
      [result("annotation", "n1", "Deploy", { url: null })],
      "admin"
    )
    expect(groups).toEqual([])
  })

  test("a comment is labelled with what it is on and shows its snippet", () => {
    const comment = result("comment", "c1", "Checkout latency", {
      snippet: [
        { text: "the ", match: false },
        { text: "checkout", match: true },
      ],
    })
    const groups = paletteGroups("checkout", [], [comment], "admin")
    expect(groups).toHaveLength(1)
    expect(groups[0]?.items[0]).toMatchObject({
      kind: "comment",
      label: "Comment on Checkout latency",
      secondary: comment.snippet,
    })
  })

  test("dashboards and saved searches show their folder, the others their snippet", () => {
    const groups = paletteGroups("checkout", [], everyKind, "admin")
    const secondary = (value: string) =>
      groups
        .find((group) => group.value === value)
        ?.items.map((entry) => ("secondary" in entry ? entry.secondary : null))
    const folder = [{ text: "Payments", match: false }]
    expect(secondary("Dashboards")).toEqual([folder])
    expect(secondary("Saved searches")).toEqual([folder])
    expect(secondary("Observations")).toEqual([
      [{ text: "about Slow pod", match: false }],
    ])
  })

  test("the path is the pathname and search of an absolute address", () => {
    const groups = paletteGroups(
      "deploy",
      [],
      [
        result("annotation", "n1", "Deploy", {
          url: "https://observe.example.com/d/d1/checkout?from=1700000000000&to=1700000900000",
        }),
      ],
      "admin"
    )
    expect(groups[0]?.items[0]).toMatchObject({
      path: "/d/d1/checkout?from=1700000000000&to=1700000900000",
    })
  })

  test("an observation opens at its own page whatever address the server sent", () => {
    const groups = paletteGroups("slow", [], everyKind, "admin")
    expect(
      groups.find((group) => group.value === "Observations")?.items
    ).toEqual([
      expect.objectContaining({
        kind: "observation",
        id: "observation:o1",
        label: "Slow pod",
        path: "/o/o1",
      }),
    ])
  })

  test("a local dashboard opens at its own address", () => {
    const groups = paletteGroups(
      "checkout",
      [{ id: "d1", title: "Checkout health" }],
      [],
      "admin"
    )
    expect(groups[0]?.items[0]).toMatchObject({ path: "/d/d1/checkout-health" })
  })

  test("pages and account actions are still filtered by the words while server results are kept whole", () => {
    const groups = paletteGroups("sign", [], everyKind, "admin")
    expect(groups.map((group) => group.value)).toEqual([
      "Dashboards",
      "Observations",
      "Comments",
      "Annotations",
      "Saved searches",
      "Account",
    ])
  })
})
