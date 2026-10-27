import { describe, expect, it } from "vitest"

import type { ApiIssue } from "@/api/errors"

import {
  rejectedQueryLines,
  showsProblem,
  splitRejected,
  withoutQuery,
  type Problem,
} from "./save-problem"

function issue(path: string, code = "query_invalid"): ApiIssue {
  return { path, code, message: `problem at ${path}` }
}

const pointer = "/panels/1"
const refs = ["A", "B"]

function split(...paths: string[]) {
  const result = splitRejected(
    paths.map((path) => issue(path)),
    pointer,
    refs
  )
  return {
    rows: Object.fromEntries(
      [...result.byQuery].map(([ref, issues]) => [
        ref,
        issues.map((item) => item.path),
      ])
    ),
    panel: result.panel.map((item) => item.path),
    dashboard: result.dashboard.map((item) => item.path),
  }
}

describe("splitRejected", () => {
  it("puts an issue under a query's pointer on that query's ref", () => {
    expect(
      split(
        "/panels/1/queries/1/query",
        "/panels/1/queries/0/datasource",
        "/panels/1/queries/1/legend"
      )
    ).toEqual({
      rows: {
        B: ["/panels/1/queries/1/query", "/panels/1/queries/1/legend"],
        A: ["/panels/1/queries/0/datasource"],
      },
      panel: [],
      dashboard: [],
    })
  })

  it("keeps issues of the panel itself apart from rows", () => {
    expect(split("/panels/1", "/panels/1/title", "/panels/1/queries")).toEqual({
      rows: {},
      panel: ["/panels/1", "/panels/1/title", "/panels/1/queries"],
      dashboard: [],
    })
  })

  it("does not take panel 10 or the dashboard for panel 1", () => {
    expect(
      split(
        "/panels/10/queries/0/query",
        "/panels/10",
        "/panels/2/title",
        "/title",
        "/panels"
      )
    ).toEqual({
      rows: {},
      panel: [],
      dashboard: [
        "/panels/10/queries/0/query",
        "/panels/10",
        "/panels/2/title",
        "/title",
        "/panels",
      ],
    })
  })

  it("splits a mixed list into rows, panel and dashboard", () => {
    expect(
      split(
        "/panels/0/queries/0/query",
        "/panels/1/queries/0/query",
        "/panels/1/gridPos",
        "/panels/10/title"
      )
    ).toEqual({
      rows: { A: ["/panels/1/queries/0/query"] },
      panel: ["/panels/1/gridPos"],
      dashboard: ["/panels/0/queries/0/query", "/panels/10/title"],
    })
  })

  it("shows a query issue with no row of its own on the panel", () => {
    expect(
      split("/panels/1/queries/2/query", "/panels/1/queries/x/query")
    ).toEqual({
      rows: {},
      panel: ["/panels/1/queries/2/query", "/panels/1/queries/x/query"],
      dashboard: [],
    })
  })

  it("leaves transient issues to the retry line", () => {
    const result = splitRejected(
      [
        issue("/panels/1/queries/0/query", "datasource_unavailable"),
        issue("/panels/3/queries/0/query", "query_timeout"),
        issue("/panels/1/queries/1/query", "internal"),
      ],
      pointer,
      refs
    )
    expect(result.byQuery.size).toBe(0)
    expect(result.panel).toEqual([])
    expect(result.dashboard).toEqual([])
  })
})

describe("rejectedQueryLines", () => {
  it("keys the lines by the ref the query had when it was saved", () => {
    const problem: Problem = {
      kind: "rejected",
      pointer,
      refs: ["B", "C"],
      issues: [issue("/panels/1/queries/1/query")],
    }
    expect([...rejectedQueryLines(problem)]).toEqual([
      ["C", ["problem at /panels/1/queries/1/query"]],
    ])
  })

  it("has no lines for any other problem", () => {
    expect(rejectedQueryLines({ kind: "gone" }).size).toBe(0)
    expect(rejectedQueryLines(null).size).toBe(0)
  })
})

describe("withoutQuery", () => {
  const rejected: Problem = {
    kind: "rejected",
    pointer,
    refs: ["A", "B"],
    issues: [
      issue("/panels/1/queries/0/query"),
      issue("/panels/1/queries/1/query"),
      issue("/panels/1/title"),
    ],
  }

  it("drops the issues of the removed query and keeps the rest", () => {
    const next = withoutQuery(rejected, "A")
    expect(next).toMatchObject({
      kind: "rejected",
      refs: [null, "B"],
      issues: [issue("/panels/1/queries/1/query"), issue("/panels/1/title")],
    })
    expect(rejectedQueryLines(next).has("A")).toBe(false)
  })

  it("clears the problem when the removed query held every issue", () => {
    const only: Problem = {
      ...rejected,
      issues: [issue("/panels/1/queries/0/query")],
    } as Problem
    expect(withoutQuery(only, "A")).toBeNull()
  })

  it("leaves other problems and unrelated removals alone", () => {
    expect(withoutQuery(rejected, "C")).toBe(rejected)
    expect(withoutQuery({ kind: "gone" }, "A")).toEqual({ kind: "gone" })
    expect(withoutQuery(null, "A")).toBeNull()
  })
})

describe("showsProblem", () => {
  const rejected = (...issues: ApiIssue[]): Problem => ({
    kind: "rejected",
    pointer,
    refs,
    issues,
  })

  it("is quiet when every issue sits in a row", () => {
    expect(showsProblem(rejected(issue("/panels/1/queries/0/query")))).toBe(
      false
    )
  })

  it("shows a rejection with nothing to place and one with panel or dashboard issues", () => {
    expect(showsProblem(rejected())).toBe(true)
    expect(
      showsProblem(
        rejected(issue("/panels/1/queries/0/query"), issue("/panels/1/title"))
      )
    ).toBe(true)
    expect(showsProblem(rejected(issue("/panels/4/title")))).toBe(true)
  })

  it("shows the retry line next to rows", () => {
    expect(
      showsProblem(
        rejected(
          issue("/panels/1/queries/0/query"),
          issue("/panels/1/queries/1/query", "query_timeout")
        )
      )
    ).toBe(true)
  })

  it("leaves a conflict to the notice and shows every other problem", () => {
    expect(showsProblem(null)).toBe(false)
    expect(showsProblem({ kind: "conflict", latest: 3 })).toBe(false)
    expect(showsProblem({ kind: "gone" })).toBe(true)
    expect(showsProblem({ kind: "transient" })).toBe(true)
    expect(showsProblem({ kind: "failed", message: "x" })).toBe(true)
  })
})
