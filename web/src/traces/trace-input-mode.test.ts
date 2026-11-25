import { act, renderHook } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import type { TraceSearch } from "./search"
import { useTraceInputMode } from "./trace-input-mode"

function setup(type: string, search: TraceSearch, agentAvailable = true) {
  return renderHook(
    (props: { search: TraceSearch }) =>
      useTraceInputMode({ type, search: props.search, agentAvailable }),
    { initialProps: { search } }
  )
}

beforeEach(() => {
  localStorage.clear()
})

describe("useTraceInputMode", () => {
  it("offers Ask and Query on Tempo", () => {
    const { result } = setup("tempo", { service: "checkout" })
    expect(result.current.modes).toEqual(["ask", "code"])
    expect(result.current.labels.code).toBe("Query")
    expect(result.current.shown).toBe("code")
    expect(result.current.sqlShown).toBe(false)
  })

  it("offers Ask, Search and SQL on ClickHouse", () => {
    const { result } = setup("clickhouse", { service: "checkout" })
    expect(result.current.modes).toEqual(["ask", "builder", "code"])
    expect(
      result.current.modes.map((mode) => result.current.labels[mode])
    ).toEqual(["Write a query", "Search", "SQL"])
    expect(result.current.shown).toBe("builder")
    expect(result.current.formMode).toBe("builder")
  })

  it("opens the SQL view when the URL carries SQL", () => {
    const { result } = setup("clickhouse", { q: "SELECT 1" })
    expect(result.current.shown).toBe("code")
    expect(result.current.sqlShown).toBe(true)
  })

  it("opens the search, not the SQL view, from a remembered query mode", () => {
    localStorage.setItem("slashobserve-input-mode", "code")
    const { result } = setup("clickhouse", {})
    expect(result.current.shown).toBe("builder")
  })

  it("names the shown ClickHouse view as the language and leaves Tempo's alone", () => {
    const { result } = setup("clickhouse", { service: "checkout" })
    expect(result.current.language).toBe("Search")
    act(() => result.current.select("code"))
    expect(result.current.language).toBe("SQL")
    act(() => result.current.select("ask"))
    expect(result.current.language).toBe("Search")
    expect(
      setup("tempo", { service: "checkout" }).result.current.language
    ).toBe(undefined)
  })

  it("switches between Search and SQL", () => {
    const { result } = setup("clickhouse", {})
    act(() => result.current.select("code"))
    expect(result.current.sqlShown).toBe(true)
    act(() => result.current.select("builder"))
    expect(result.current.shown).toBe("builder")
    act(() => result.current.select("ask"))
    expect(result.current.shown).toBe("ask")
  })

  it("follows the URL when a search replaces the SQL", () => {
    const { result, rerender } = setup("clickhouse", { q: "SELECT 1" })
    rerender({ search: { service: "checkout" } })
    expect(result.current.shown).toBe("builder")
    rerender({ search: { q: "SELECT 2" } })
    expect(result.current.shown).toBe("code")
  })

  it("cycles Search, SQL and Ask with the shortcut", () => {
    const { result } = setup("clickhouse", { service: "checkout" })
    expect(result.current.following()).toBe("code")
    act(() => result.current.select("code"))
    expect(result.current.following()).toBe("ask")
    act(() => result.current.select("ask"))
    expect(result.current.following()).toBe("builder")
  })

  it("skips Ask in the cycle without a model", () => {
    const { result } = setup("clickhouse", { q: "SELECT 1" }, false)
    expect(result.current.following()).toBe("builder")
  })
})
