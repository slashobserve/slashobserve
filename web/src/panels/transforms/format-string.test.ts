import { describe, expect, it } from "vitest"

import { formatString } from "./format-string"
import type { PreparedFrame } from "./names"

type Format = Parameters<typeof formatString>[1]["format"]

function run(
  format: Format,
  values: unknown[],
  extra: { start?: number; end?: number } = {}
): unknown[] {
  const frame: PreparedFrame = {
    ref: "A",
    fields: [
      { name: "n", type: "number", values: [1, 2] },
      { name: "s", type: "string", values },
    ],
  }
  const [out] = formatString([frame], {
    type: "formatString",
    field: "s",
    format,
    ...extra,
  })
  return out.fields[1].values
}

describe("formatString", () => {
  it.each<[Format, string, string]>([
    ["upper", "Hello wORLD", "HELLO WORLD"],
    ["lower", "Hello wORLD", "hello world"],
    ["sentence", "hELLO wORLD", "Hello world"],
    ["title", "hELLO   wORLD again", "Hello   World Again"],
    ["pascal", "hello_world-foo bar", "HelloWorldFooBar"],
    ["camel", "Hello_world-foo bar", "helloWorldFooBar"],
    ["snake", "helloWorld Foo-bar", "hello_world_foo_bar"],
    ["kebab", "helloWorld Foo_bar", "hello-world-foo-bar"],
    ["trim", "  padded \n", "padded"],
  ])("%s writes %j as %j", (format, input, expected) => {
    expect(run(format, [input])).toEqual([expected])
  })

  it("cuts a substring like slice and leaves other values alone", () => {
    expect(run("substring", ["abcdef", null, 7], { start: 1, end: 4 })).toEqual(
      ["bcd", null, 7]
    )
    expect(run("substring", ["abcdef"], { start: 2 })).toEqual(["cdef"])
    expect(run("substring", ["abcdef"])).toEqual(["abcdef"])
  })

  it("leaves other fields and unknown field names alone", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [{ name: "s", type: "string", values: ["a"] }],
    }
    const [out] = formatString([frame], {
      type: "formatString",
      field: "other",
      format: "upper",
    })
    expect(out).toEqual(frame)
  })

  it("handles accented letters in word formats", () => {
    expect(run("snake", ["ÉcoleNormale supérieure"])).toEqual([
      "école_normale_supérieure",
    ])
  })
})
