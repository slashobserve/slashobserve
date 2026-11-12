import { describe, expect, test } from "bun:test"
import {
  Pruner,
  cutResult,
  keptBytes,
  tailStart,
  type PruneMessage,
  type Weighed,
} from "../../src/session/mechanics/compact"

describe("cutResult", () => {
  const cases: Array<[string, string, number, string]> = [
    ["short content is untouched", "abc", 10, "abc"],
    ["exactly the limit is untouched", "abcd", 4, "abcd"],
    [
      "ascii is cut at the byte limit",
      "abcdefghij",
      4,
      "abcd\n[cut from 10 bytes; call the tool again for the full result]",
    ],
    [
      "a multi-byte rune is not split",
      "abécd",
      3,
      "ab\n[cut from 6 bytes; call the tool again for the full result]",
    ],
    ["a zero limit cuts nothing", "abcdef", 0, "abcdef"],
    [
      "a four byte rune is not split",
      "a😀b",
      3,
      "a\n[cut from 6 bytes; call the tool again for the full result]",
    ],
  ]
  test.each(cases)("%s", (_name, content, limit, want) => {
    const got = cutResult(content, limit)
    expect(got).toBe(want)
    expect(got).not.toContain("�")
  })
})

describe("tailStart", () => {
  const a = (bytes: number): Weighed => ({ assistant: true, bytes })
  const u = (bytes: number): Weighed => ({ assistant: false, bytes })
  const history = [a(10), u(10), a(10), u(10), a(10), u(10), a(10)]
  const cases: Array<[string, Weighed[], number, number, number]> = [
    ["no messages", [], 4, 1000, 0],
    ["no assistant message", [u(5)], 4, 1000, 1],
    ["the last assistant is always kept", history, 1, 1000, 6],
    ["two turns", history, 2, 1000, 4],
    ["more turns than exist keeps all", history, 9, 1000, 0],
    ["the byte limit stops before a turn that does not fit", history, 4, 35, 4],
    ["the last assistant is kept even over the byte limit", history, 4, 1, 6],
    ["zero turns still keeps the last assistant", history, 0, 1000, 6],
  ]
  test.each(cases)("%s", (_name, messages, turns, bytes, want) => {
    expect(tailStart(messages, turns, bytes)).toBe(want)
  })
})

test("keptBytes caps at the result limit", () => {
  expect(keptBytes(5000, 100)).toBe(100)
  expect(keptBytes(50, 100)).toBe(50)
  expect(keptBytes(5000, 0)).toBe(5000)
})

describe("Pruner", () => {
  const assistant: PruneMessage = { assistant: true, results: [] }
  const results = (id: string, bytes: number): PruneMessage => ({
    assistant: false,
    results: [{ id, bytes }],
  })
  const history: PruneMessage[] = [
    { assistant: false, results: [] },
    assistant,
    results("a", 600),
    assistant,
    results("b", 600),
    assistant,
    results("c", 600),
    assistant,
  ]
  const cases: Array<
    [string, { keepTurns: number; stepBytes: number }, string[], number]
  > = [
    [
      "below the step nothing moves",
      { keepTurns: 2, stepBytes: 2000 },
      ["a", "b", "c"],
      0,
    ],
    [
      "the step passes the boundary before the kept turns",
      { keepTurns: 2, stepBytes: 1000 },
      ["a", "b", "c"],
      5,
    ],
    [
      "results that are not prunable do not count",
      { keepTurns: 2, stepBytes: 1000 },
      ["a"],
      0,
    ],
    [
      "fewer assistant turns than kept turns moves nothing",
      { keepTurns: 9, stepBytes: 1 },
      ["a", "b", "c"],
      0,
    ],
    ["zero pruning is off", { keepTurns: 0, stepBytes: 0 }, ["a", "b", "c"], 0],
  ]
  test.each(cases)("%s", (_name, config, prunable, want) => {
    const pruner = new Pruner(config)
    pruner.note(prunable.map((callId) => ({ callId, prunable: true })))
    expect(pruner.boundary(history)).toBe(want)
    if (want > 0) {
      pruner.reset()
      expect(pruner.boundary(history.slice(0, 2))).toBe(0)
    }
  })

  test("only results noted as prunable are prunable", () => {
    const pruner = new Pruner({ keepTurns: 2, stepBytes: 1 })
    pruner.note([
      { callId: "a", prunable: true },
      { callId: "b", prunable: false },
    ])
    expect(pruner.prunable("a")).toBe(true)
    expect(pruner.prunable("b")).toBe(false)
  })
})
