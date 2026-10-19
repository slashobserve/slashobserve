import { describe, expect, it } from "vitest"

import {
  countText,
  hasFilters,
  madeByText,
  toFilters,
  validateListSearch,
} from "./list-filters"

const id = "0b6a1f3e-7c1d-4f0e-9a52-1d2c3b4a5e6f"

describe("validateListSearch", () => {
  it("keeps valid values", () => {
    expect(
      validateListSearch({
        q: "  checkout ",
        made_by: "me",
        datasource: "prom",
      })
    ).toEqual({ q: "checkout", made_by: "me", datasource: "prom" })
  })

  it("takes the first value of a repeated key", () => {
    expect(
      validateListSearch({ q: ["a", "b"], datasource: ["x", "y"] })
    ).toEqual({ q: "a", datasource: "x" })
  })

  it("drops a query that is empty, too long or holds a NUL", () => {
    expect(validateListSearch({ q: "   " })).toEqual({})
    expect(validateListSearch({ q: "a".repeat(201) })).toEqual({})
    expect(validateListSearch({ q: "a\0b" })).toEqual({})
    expect(validateListSearch({ q: 7 })).toEqual({})
  })

  it("counts code points, not UTF-16 units, for the query limit", () => {
    expect(validateListSearch({ q: "😀".repeat(200) }).q).toBeDefined()
    expect(validateListSearch({ q: "😀".repeat(201) })).toEqual({})
  })

  it("accepts the made_by forms and drops the rest", () => {
    for (const value of [
      "me",
      "agent",
      "mcp",
      "mcp:claude-code",
      `user:${id}`,
      `service_account:${id}`,
    ]) {
      expect(validateListSearch({ made_by: value })).toEqual({ made_by: value })
    }
    for (const value of [
      "",
      "anyone",
      "mcp:",
      "mcp: x",
      `mcp:${"a".repeat(65)}`,
      "user:abc",
      `user:${id.toUpperCase()}`,
      "service_account:",
    ]) {
      expect(validateListSearch({ made_by: value })).toEqual({})
    }
  })

  it("limits the datasource to 1 to 64 characters", () => {
    expect(validateListSearch({ datasource: "" })).toEqual({})
    expect(validateListSearch({ datasource: "a".repeat(65) })).toEqual({})
    expect(validateListSearch({ datasource: "a".repeat(64) }).datasource).toBe(
      "a".repeat(64)
    )
  })
})

describe("toFilters and hasFilters", () => {
  it("renames made_by", () => {
    expect(toFilters({ q: "a", made_by: "agent", datasource: "ch" })).toEqual({
      q: "a",
      madeBy: "agent",
      datasource: "ch",
    })
  })

  it("reports whether any filter is set", () => {
    expect(hasFilters({})).toBe(false)
    expect(hasFilters({ q: "a" })).toBe(true)
    expect(hasFilters({ made_by: "me" })).toBe(true)
    expect(hasFilters({ datasource: "prom" })).toBe(true)
  })
})

describe("madeByText", () => {
  const users = [{ id, name: "Ishan" }]

  it("describes each filter value", () => {
    expect(madeByText(undefined, users)).toBe("anyone")
    expect(madeByText("me", users)).toBe("me")
    expect(madeByText("agent", users)).toBe("Agent")
    expect(madeByText("mcp", users)).toBe("MCP clients")
    expect(madeByText("mcp:claude-code", users)).toBe("Claude Code")
    expect(madeByText("mcp:my-tool", users)).toBe("my-tool")
    expect(madeByText(`user:${id}`, users)).toBe("Ishan")
    expect(madeByText(`service_account:${id}`, users)).toBe("a service account")
  })

  it("names an unknown person generically", () => {
    expect(madeByText(`user:${id}`, [])).toBe("a person")
  })
})

describe("countText", () => {
  it("pluralises", () => {
    expect(countText(0)).toBe("0 dashboards")
    expect(countText(1)).toBe("1 dashboard")
    expect(countText(6)).toBe("6 dashboards")
  })
})
