import { describe, expect, it } from "vitest"

import {
  normalizePromptName,
  promptBodyError,
  promptNameError,
  suggestPromptName,
} from "./rules"

describe("normalizePromptName", () => {
  it("trims and collapses runs of whitespace", () => {
    expect(normalizePromptName("  Checkout \t  health \n")).toBe(
      "Checkout health"
    )
    expect(normalizePromptName("   ")).toBe("")
  })
})

describe("promptNameError", () => {
  const cases: { name: string; input: string; expected: string | null }[] = [
    { name: "an empty name", input: "   ", expected: "Enter a name." },
    {
      name: "a name over 64 characters",
      input: "a".repeat(65),
      expected: "Use at most 64 characters.",
    },
    { name: "a name of exactly 64", input: "a".repeat(64), expected: null },
    {
      name: "64 astral characters counted as characters",
      input: "\u{1D4D0}".repeat(64),
      expected: null,
    },
    {
      name: "a leading dash",
      input: "-health",
      expected:
        "Use letters, numbers, spaces, - and _, starting and ending with a letter or number.",
    },
    {
      name: "a trailing underscore",
      input: "health_",
      expected:
        "Use letters, numbers, spaces, - and _, starting and ending with a letter or number.",
    },
    {
      name: "a punctuation character",
      input: "p99: latency",
      expected:
        "Use letters, numbers, spaces, - and _, starting and ending with a letter or number.",
    },
    { name: "a plain name", input: "Checkout health", expected: null },
    {
      name: "a name with dash and underscore",
      input: "a-b_c d",
      expected: null,
    },
    { name: "a single character", input: "x", expected: null },
    { name: "non-ASCII letters", input: "Prüfung Übersicht", expected: null },
  ]

  it.each(cases)("judges $name", ({ input, expected }) => {
    expect(promptNameError(input)).toBe(expected)
  })
})

describe("promptBodyError", () => {
  it("rejects empty and over-long text and accepts the rest", () => {
    expect(promptBodyError("  \n ")).toBe("Enter the prompt.")
    expect(promptBodyError("a".repeat(4001))).toBe(
      "Use at most 4000 characters."
    )
    expect(promptBodyError("a".repeat(4000))).toBeNull()
    expect(promptBodyError(` ${"a".repeat(4000)} `)).toBeNull()
  })
})

describe("suggestPromptName", () => {
  const cases: { name: string; input: string; expected: string }[] = [
    {
      name: "the example prompt",
      input: "checkout service health by pod, last 6h",
      expected: "Checkout service health",
    },
    { name: "a lone slash", input: "/", expected: "" },
    { name: "blank text", input: "   ", expected: "" },
    {
      name: "a 100 character single word",
      input: "a".repeat(100),
      expected: `A${"a".repeat(63)}`,
    },
    {
      name: "punctuation between words",
      input: "p99 latency: checkout",
      expected: "P99 latency checkout",
    },
    {
      name: "only the first line",
      input: "error budget\nburn rate for payments",
      expected: "Error budget",
    },
    {
      name: "leading and trailing separators",
      input: "--- cpu usage ---",
      expected: "Cpu usage",
    },
    {
      name: "long words cut at a word boundary",
      input: `${"a".repeat(40)} ${"b".repeat(40)} tail`,
      expected: `A${"a".repeat(39)}`,
    },
  ]

  it.each(cases)("suggests for $name", ({ input, expected }) => {
    expect(suggestPromptName(input)).toBe(expected)
  })

  it("only suggests names the name rule accepts", () => {
    for (const input of [
      "checkout service health by pod, last 6h",
      "p99 latency: checkout",
      "_-_ a _-_",
    ]) {
      const name = suggestPromptName(input)
      expect(promptNameError(name)).toBeNull()
    }
  })
})
