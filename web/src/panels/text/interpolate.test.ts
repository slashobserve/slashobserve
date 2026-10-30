import { describe, expect, it } from "vitest"

import { ALL } from "@/dashboards/search"

import { interpolateText, type TextMode } from "./interpolate"

describe("interpolateText", () => {
  it("joins every selected value of a braced variable with a comma", () => {
    expect(
      interpolateText("Env: ${env}", { env: ["prod", "staging"] }, "markdown")
    ).toBe("Env: prod, staging")
  })

  it("accepts the raw format suffix", () => {
    expect(interpolateText("${env:raw}", { env: ["prod"] }, "markdown")).toBe(
      "prod"
    )
  })

  it("replaces a bare variable at a word boundary", () => {
    expect(interpolateText("in $env.", { env: ["prod"] }, "markdown")).toBe(
      "in prod."
    )
  })

  it("does not replace a bare variable inside a longer name", () => {
    expect(
      interpolateText("$envx and $env", { env: ["prod"] }, "markdown")
    ).toBe("$envx and prod")
  })

  it("reads an All selection as All", () => {
    expect(interpolateText("Pods: ${pod}", { pod: [ALL] }, "markdown")).toBe(
      "Pods: All"
    )
  })

  it("keeps unknown variables as written", () => {
    expect(
      interpolateText("${nope} $nope ${__from}", { env: ["a"] }, "markdown")
    ).toBe("${nope} $nope ${__from}")
  })

  it("does not read inherited names as variables", () => {
    expect(interpolateText("${constructor} $toString", {}, "markdown")).toBe(
      "${constructor} $toString"
    )
  })

  it("keeps a link written in a value from becoming a link", () => {
    const value = "[x](javascript:alert(1))"
    const out = interpolateText("${env}", { env: [value] }, "markdown")
    expect(out).toBe("\\[x\\]\\(javascript\\:alert\\(1\\)\\)")
    expect(out).not.toContain("[x]")
  })

  it("escapes every ASCII punctuation character in a value", () => {
    const punctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
    const out = interpolateText("${v}", { v: [punctuation] }, "markdown")
    expect(out.replaceAll("⁠", "")).toBe(
      "\\!&quot;\\#\\$\\%&amp;&#39;\\(\\)\\*\\+\\,\\-\\.\\/\\:\\;&lt;\\=&gt;\\?\\@\\[\\\\\\]\\^\\_\\`\\{\\|\\}\\~"
    )
  })

  it.each([
    ["https://evil.example", "https\\:\u2060\\/\\/evil\\.example"],
    ["www.evil.example", "www\u2060\\.evil\\.example"],
    ["a@evil.example", "a\\@\u2060evil\\.example"],
  ])(
    "breaks the web address or email %s so it cannot autolink",
    (value, out) => {
      expect(interpolateText("${v}", { v: [value] }, "markdown")).toBe(out)
    }
  )

  it("leaves ordinary values free of invisible characters", () => {
    expect(
      interpolateText("${v}", { v: ["checkout-0", "v1.5"] }, "markdown")
    ).toBe("checkout\\-0, v1\\.5")
  })

  it("escapes each value before joining", () => {
    expect(interpolateText("${v}", { v: ["a_b", "*c*"] }, "markdown")).toBe(
      "a\\_b, \\*c\\*"
    )
  })

  it("collapses line breaks in a value so it cannot start a block", () => {
    expect(
      interpolateText("${v}", { v: ["a\n\n# title\n- item"] }, "markdown")
    ).toBe("a \\# title \\- item")
  })

  it("does not substitute inside an inserted value again", () => {
    expect(interpolateText("${a}", { a: ["${b}"], b: ["x"] }, "markdown")).toBe(
      "\\$\\{b\\}"
    )
  })

  it("leaves text without variables unchanged", () => {
    expect(interpolateText("# Title\n\ncost: 5$", {}, "markdown")).toBe(
      "# Title\n\ncost: 5$"
    )
  })

  it.each<[TextMode, string]>([
    ["markdown", "&lt;b&gt;x&lt;\\/b&gt;"],
    ["html", "&lt;b&gt;x&lt;/b&gt;"],
    ["code", "<b>x</b>"],
  ])("writes a tag in a value as %s mode text", (mode, out) => {
    expect(interpolateText("${v}", { v: ["<b>x</b>"] }, mode)).toBe(out)
  })

  it.each<[TextMode, string]>([
    ["markdown", "a&amp;b &quot;c&quot; &#39;d&#39;"],
    ["html", "a&amp;b &quot;c&quot; &#39;d&#39;"],
    ["code", "a&b \"c\" 'd'"],
  ])("handles ampersands and quotes in %s mode", (mode, out) => {
    expect(interpolateText("${v}", { v: ["a&b \"c\" 'd'"] }, mode)).toBe(out)
  })

  it.each<TextMode>(["markdown", "html", "code"])(
    "reads an All selection as All in %s mode",
    (mode) => {
      expect(interpolateText("${v}", { v: [ALL] }, mode)).toBe("All")
    }
  )

  it.each<TextMode>(["html", "code"])(
    "keeps whitespace and punctuation of a value in %s mode",
    (mode) => {
      expect(interpolateText("${v}", { v: ["a  b\nc_d"] }, mode)).toBe(
        "a  b\nc_d"
      )
    }
  )
})
