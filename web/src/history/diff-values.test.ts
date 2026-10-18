import { describe, expect, test } from "vitest"

import {
  formatValue,
  limitBlocks,
  shouldLabelQueries,
  type ValueBlock,
} from "./diff-values"

const titles = { from: {}, to: {} }

function format(
  value: unknown,
  label = shouldLabelQueries("queries", false, value)
) {
  return formatValue("queries", value, titles, false, label)
}

describe("formatValue for queries", () => {
  test("a single query without a ref has no label", () => {
    expect(format([{ query: "up" }])).toEqual([
      { label: undefined, text: "up", options: "" },
    ])
  })

  test("several queries are labelled by position", () => {
    expect(
      format([{ query: "a" }, { query: "b" }]).map((b) => b.label)
    ).toEqual(["Query A", "Query B"])
  })

  test("an empty ref counts as missing", () => {
    expect(format([{ ref: "", query: "a" }])[0].label).toBeUndefined()
    expect(
      format([{ ref: "", query: "a" }, { query: "b" }]).map((b) => b.label)
    ).toEqual(["Query A", "Query B"])
    expect(
      shouldLabelQueries("queries", false, [{ ref: "", query: "a" }])
    ).toBe(false)
  })

  test("an explicit ref labels a single query and wins over position", () => {
    expect(format([{ ref: "Z", query: "a" }])[0].label).toBe("Query Z")
    expect(
      format([{ query: "a" }, { ref: "X", query: "b" }]).map((b) => b.label)
    ).toEqual(["Query A", "Query X"])
  })

  test("a shared flag labels a single query on the other side", () => {
    expect(format([{ query: "a" }], true)[0].label).toBe("Query A")
  })

  test("options list the other keys, with hide as hidden", () => {
    const [block] = format([
      {
        ref: "A",
        query: "up",
        legend: "{{pod}}",
        instant: true,
        hide: true,
        interval: "30s",
        format: "table",
      },
    ])
    expect(block.options).toBe(
      "legend: {{pod}}, instant, hidden, interval: 30s, format: table"
    )
    expect(
      format([{ query: "up", hide: false, legend: undefined }])[0].options
    ).toBe("")
    expect(format([{ query: "up", hide: true }])[0].options).toBe("hidden")
  })

  test("an empty query keeps an empty text", () => {
    expect(format([{ query: "" }])[0].text).toBe("")
  })

  test.each([
    ["a missing query", [{ legend: "x" }]],
    ["a non-string query", [{ query: 1 }]],
    ["a non-string ref", [{ ref: 1, query: "up" }]],
    ["a null entry", [null]],
    ["an array entry", [["up"]]],
    ["one malformed entry among valid ones", [{ query: "up" }, { query: 2 }]],
    ["an empty list", []],
  ])("falls back to JSON for %s", (_, value) => {
    const blocks = format(value)
    expect(blocks).toHaveLength(1)
    expect(blocks[0].label).toBeUndefined()
    expect(blocks[0].text).toBe(JSON.stringify(value, null, 2))
  })

  test("other fields are plain text or JSON", () => {
    expect(formatValue("title", "Hello", titles, false, false)).toEqual([
      { text: "Hello", options: "" },
    ])
    expect(formatValue("title", undefined, titles, false, false)[0].text).toBe(
      ""
    )
    expect(formatValue("tags", ["a"], titles, false, false)[0].text).toBe(
      '[\n  "a"\n]'
    )
  })
})

describe("shouldLabelQueries", () => {
  test("is true when either side has several queries or an explicit ref", () => {
    const one = [{ query: "a" }]
    const two = [{ query: "a" }, { query: "b" }]
    expect(shouldLabelQueries("queries", false, one)).toBe(false)
    expect(shouldLabelQueries("queries", false, one, two)).toBe(true)
    expect(
      shouldLabelQueries("queries", false, undefined, [
        { ref: "A", query: "a" },
      ])
    ).toBe(true)
  })

  test("is false for other fields, reorders and malformed values", () => {
    const two = [{ query: "a" }, { query: "b" }]
    expect(shouldLabelQueries("tags", false, two)).toBe(false)
    expect(shouldLabelQueries("queries", true, two)).toBe(false)
    expect(
      shouldLabelQueries("queries", false, [
        { ref: 1, query: "a" },
        { query: "b" },
      ])
    ).toBe(false)
  })
})

describe("formatValue for reorders", () => {
  test("numbers the new titles, falling back to old titles and ids", () => {
    const blocks = formatValue(
      "panels",
      ["a", "b", "c"],
      { from: { b: "Old B", c: "Old C" }, to: { a: "New A", c: "New C" } },
      true,
      false
    )
    expect(blocks).toEqual([
      { text: "1. New A\n2. Old B\n3. New C", options: "" },
    ])
  })

  test("does not read inherited keys as titles", () => {
    const [block] = formatValue("panels", ["constructor"], titles, true, false)
    expect(block.text).toBe("1. constructor")
  })

  test("a non-array reorder value is plain text", () => {
    expect(formatValue("panels", "x", titles, true, false)[0].text).toBe("x")
  })
})

describe("limitBlocks", () => {
  const block = (text: string, options = "", label?: string): ValueBlock => ({
    label,
    text,
    options,
  })

  test("keeps blocks within the limit", () => {
    const blocks = [block("abc", "d"), block("ef")]
    expect(limitBlocks(blocks, 6)).toEqual({ blocks, cut: false })
  })

  test("cuts one long text", () => {
    expect(limitBlocks([block("abcdef")], 4)).toEqual({
      blocks: [block("abcd...")],
      cut: true,
    })
  })

  test("counts the limit across blocks and hides the rest", () => {
    const blocks = [
      block("aaa", "", "Query A"),
      block("bbb", "", "Query B"),
      block("ccc", "", "Query C"),
    ]
    const result = limitBlocks(blocks, 5)
    expect(result.cut).toBe(true)
    expect(result.blocks).toEqual([blocks[0], { ...blocks[1], text: "bb..." }])
  })

  test("drops later blocks once the limit is used exactly", () => {
    const result = limitBlocks([block("aaa"), block("bbb")], 3)
    expect(result).toEqual({ blocks: [block("aaa")], cut: true })
  })

  test("counts the options line and cuts it", () => {
    expect(limitBlocks([block("abc", "legend: long")], 6)).toEqual({
      blocks: [block("abc", "leg...")],
      cut: true,
    })
    expect(limitBlocks([block("abc", "legend")], 3)).toEqual({
      blocks: [block("abc", "...")],
      cut: true,
    })
  })

  test("keeps the options line when a long text is cut", () => {
    const long = "q".repeat(100)
    const result = limitBlocks([block(long, "legend: b")], 50)
    expect(result).toEqual({
      blocks: [block(`${"q".repeat(41)}...`, "legend: b")],
      cut: true,
    })
  })

  test("cuts the options line when it fills the whole limit", () => {
    expect(limitBlocks([block("abcdef", "legend: long")], 4).blocks).toEqual([
      block("...", "lege..."),
    ])
  })

  test("stops at exactly zero remaining, even before an empty block", () => {
    expect(limitBlocks([block("aaa"), block("")], 3)).toEqual({
      blocks: [block("aaa")],
      cut: true,
    })
  })
})
