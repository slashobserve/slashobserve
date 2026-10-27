import { describe, expect, it } from "vitest"

import { ALL } from "@/dashboards/search"

import { interpolateText } from "./interpolate"

describe("interpolateText", () => {
  it("joins every selected value of a braced variable with a comma", () => {
    expect(interpolateText("Env: ${env}", { env: ["prod", "staging"] })).toBe(
      "Env: prod, staging"
    )
  })

  it("accepts the raw format suffix", () => {
    expect(interpolateText("${env:raw}", { env: ["prod"] })).toBe("prod")
  })

  it("replaces a bare variable at a word boundary", () => {
    expect(interpolateText("in $env.", { env: ["prod"] })).toBe("in prod.")
  })

  it("does not replace a bare variable inside a longer name", () => {
    expect(interpolateText("$envx and $env", { env: ["prod"] })).toBe(
      "$envx and prod"
    )
  })

  it("reads an All selection as All", () => {
    expect(interpolateText("Pods: ${pod}", { pod: [ALL] })).toBe("Pods: All")
  })

  it("keeps unknown variables as written", () => {
    expect(interpolateText("${nope} $nope ${__from}", { env: ["a"] })).toBe(
      "${nope} $nope ${__from}"
    )
  })

  it("does not read inherited names as variables", () => {
    expect(interpolateText("${constructor} $toString", {})).toBe(
      "${constructor} $toString"
    )
  })

  it("keeps a link written in a value from becoming a link", () => {
    const value = "[x](javascript:alert(1))"
    const out = interpolateText("${env}", { env: [value] })
    expect(out).toBe("\\[x\\]\\(javascript\\:alert\\(1\\)\\)")
    expect(out).not.toContain("[x]")
  })

  it("escapes every ASCII punctuation character in a value", () => {
    const punctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
    const out = interpolateText("${v}", { v: [punctuation] }).replaceAll(
      "\u2060",
      ""
    )
    expect(out).toBe([...punctuation].map((char) => `\\${char}`).join(""))
  })

  it.each([
    ["https://evil.example", "https\\:\u2060\\/\\/evil\\.example"],
    ["www.evil.example", "www\u2060\\.evil\\.example"],
    ["a@evil.example", "a\\@\u2060evil\\.example"],
  ])(
    "breaks the web address or email %s so it cannot autolink",
    (value, out) => {
      expect(interpolateText("${v}", { v: [value] })).toBe(out)
    }
  )

  it("leaves ordinary values free of invisible characters", () => {
    expect(interpolateText("${v}", { v: ["checkout-0", "v1.5"] })).toBe(
      "checkout\\-0, v1\\.5"
    )
  })

  it("escapes each value before joining", () => {
    expect(interpolateText("${v}", { v: ["a_b", "*c*"] })).toBe(
      "a\\_b, \\*c\\*"
    )
  })

  it("collapses line breaks in a value so it cannot start a block", () => {
    expect(interpolateText("${v}", { v: ["a\n\n# title\n- item"] })).toBe(
      "a \\# title \\- item"
    )
  })

  it("does not substitute inside an inserted value again", () => {
    expect(interpolateText("${a}", { a: ["${b}"], b: ["x"] })).toBe(
      "\\$\\{b\\}"
    )
  })

  it("leaves text without variables unchanged", () => {
    expect(interpolateText("# Title\n\ncost: 5$", {})).toBe(
      "# Title\n\ncost: 5$"
    )
  })
})
