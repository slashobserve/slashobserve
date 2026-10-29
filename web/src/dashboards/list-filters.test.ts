import { describe, expect, it } from "vitest"

import {
  clampQuery,
  folderText,
  hasFilters,
  madeByText,
  maxQueryLength,
  maxTags,
  tagsText,
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
    ).toEqual(["datasource", "folder", "made_by", "q", "tag"])
  })

  it("keeps a folder id in lower case and drops anything else", () => {
    expect(validateListSearch({ folder: id.toUpperCase() })).toEqual({
      folder: id,
    })
    expect(validateListSearch({ folder: [id, "x"] })).toEqual({ folder: id })
    for (const value of ["", "general", `${id}x`, ` ${id}`, 7, null]) {
      expect(validateListSearch({ folder: value })).toEqual({})
    }
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

  it("lowercases the id of a person or service account", () => {
    expect(validateListSearch({ made_by: `user:${id.toUpperCase()}` })).toEqual(
      {
        made_by: `user:${id}`,
      }
    )
    expect(
      validateListSearch({ made_by: `service_account:${id.toUpperCase()}` })
    ).toEqual({ made_by: `service_account:${id}` })
    expect(validateListSearch({ made_by: "mcp:Claude-Code" }).made_by).toBe(
      "mcp:Claude-Code"
    )
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

describe("validateListSearch tags", () => {
  it("reads a single tag or a repeated tag", () => {
    expect(validateListSearch({ tag: "checkout" }).tag).toEqual(["checkout"])
    expect(validateListSearch({ tag: ["prod", "checkout"] }).tag).toEqual([
      "checkout",
      "prod",
    ])
  })

  it("drops invalid and duplicate values", () => {
    expect(
      validateListSearch({
        tag: [
          "Prod",
          "-x",
          "a b",
          "",
          7,
          "a".repeat(41),
          "ok",
          "ok",
          "v1.2_x-y",
        ],
      }).tag
    ).toEqual(["ok", "v1.2_x-y"])
    expect(validateListSearch({ tag: ["BAD", ""] }).tag).toBeUndefined()
    expect(validateListSearch({ tag: 7 }).tag).toBeUndefined()
    expect(validateListSearch({}).tag).toBeUndefined()
  })

  it("keeps a 40 character tag", () => {
    expect(validateListSearch({ tag: "a".repeat(40) }).tag).toEqual([
      "a".repeat(40),
    ])
  })

  it("cuts more than the maximum to the first sorted ones", () => {
    const tags = Array.from(
      { length: 12 },
      (_, i) => `t${String(i).padStart(2, "0")}`
    )
    const kept = validateListSearch({ tag: [...tags].reverse() }).tag
    expect(maxTags).toBe(10)
    expect(kept).toEqual(tags.slice(0, 10))
  })

  it("normalizes the order", () => {
    expect(validateListSearch({ tag: ["b", "c", "a"] }).tag).toEqual([
      "a",
      "b",
      "c",
    ])
  })
})

describe("tagsText", () => {
  it("describes the selection", () => {
    expect(tagsText(undefined)).toBe("any")
    expect(tagsText([])).toBe("any")
    expect(tagsText(["checkout"])).toBe("checkout")
    expect(tagsText(["checkout", "prod"])).toBe("checkout +1")
    expect(tagsText(["a", "b", "c"])).toBe("a +2")
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

  it("passes the folder through", () => {
    expect(toFilters({ folder: id }).folder).toBe(id)
  })

  it("passes tags through", () => {
    expect(toFilters({ tag: ["a", "b"] }).tags).toEqual(["a", "b"])
  })

  it("reports whether any filter is set", () => {
    expect(hasFilters({})).toBe(false)
    expect(hasFilters({ q: "a" })).toBe(true)
    expect(hasFilters({ made_by: "me" })).toBe(true)
    expect(hasFilters({ datasource: "prom" })).toBe(true)
    expect(hasFilters({ tag: ["a"] })).toBe(true)
    expect(hasFilters({ folder: id })).toBe(true)
    expect(hasFilters({ tag: undefined })).toBe(false)
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

describe("folderText", () => {
  const folders = [{ id, title: "Checkout" }]

  it("describes the folder filter", () => {
    expect(folderText(undefined, folders)).toBe("any")
    expect(folderText(id, folders)).toBe("Checkout")
  })

  it("names a folder it cannot find generically", () => {
    expect(folderText(id, [])).toBe("a folder")
  })
})
