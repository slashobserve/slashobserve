import { describe, expect, it } from "vitest"

import {
  clampQuery,
  countText,
  hasFilters,
  madeByText,
  maxQueryLength,
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

  it("names every filter so a dropped value overrides the raw URL", () => {
    expect(
      Object.keys(validateListSearch({ made_by: "bogus" })).sort()
    ).toEqual(["datasource", "made_by", "q"])
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
      "mcp:Claude Code",
      `mcp:${"a".repeat(64)}`,
      `mcp:${"😀".repeat(64)}`,
      `user:${id}`,
      `user:${id.toUpperCase()}`,
      `service_account:${id}`,
    ]) {
      expect(validateListSearch({ made_by: value })).toEqual({ made_by: value })
    }
    for (const value of [
      "",
      "anyone",
      "mcp:",
      "user:abc",
      "service_account:",
      `user:${"-".repeat(36)}`,
      `user:${id.replaceAll("-", "")}-abcd`,
      `user:${id}0`,
      `user:${id.slice(0, -1)}g`,
      `service_account:${"0".repeat(36)}`,
      `robot:${id}`,
    ]) {
      expect(validateListSearch({ made_by: value })).toEqual({})
    }
  })

  it("holds an MCP client name to what the server accepts", () => {
    for (const client of [
      " x",
      "x ",
      "\tx",
      "x\n",
      "\u00a0x",
      "x\u3000",
      "a\u0001b",
      "a\nb",
      "a\u007fb",
      "a\u200bb",
      "a\u202eb",
      "a\ufeffb",
      "a\ud800b",
      "a".repeat(65),
      "😀".repeat(65),
    ]) {
      expect(validateListSearch({ made_by: `mcp:${client}` })).toEqual({})
    }
  })

  it("keeps only datasource uids the registry accepts", () => {
    for (const uid of [
      "p",
      "prom",
      "ch_tight",
      "a-b_c",
      "A1",
      "a".repeat(40),
    ]) {
      expect(validateListSearch({ datasource: uid }).datasource).toBe(uid)
    }
    for (const uid of [
      "",
      "a".repeat(41),
      "_prom",
      "-prom",
      "pro m",
      "prom\n",
      "pro\0m",
      "pröm",
      "prom.1",
    ]) {
      expect(validateListSearch({ datasource: uid })).toEqual({})
    }
  })
})

describe("clampQuery", () => {
  it("cuts text to the validator's limit in code points", () => {
    const text = "😀".repeat(maxQueryLength + 5)
    expect(Array.from(clampQuery(text))).toHaveLength(maxQueryLength)
    expect(validateListSearch({ q: clampQuery(text) }).q).toBeDefined()
    expect(clampQuery("short")).toBe("short")
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
