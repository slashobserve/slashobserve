import { describe, expect, it } from "vitest"

import { extractFields } from "./extract-fields"
import type { PreparedFrame } from "./names"

type Options = Parameters<typeof extractFields>[1]

function frameOf(lines: unknown[]): PreparedFrame {
  return {
    ref: "A",
    fields: [
      { name: "Time", type: "time", values: lines.map((_, i) => i) },
      { name: "line", type: "string", values: lines },
    ],
  }
}

function run(
  lines: unknown[],
  extra: Partial<Options> = {}
): Record<string, { type: string; values: unknown[] }> {
  const [out] = extractFields([frameOf(lines)], {
    type: "extractFields",
    field: "line",
    ...extra,
  })
  return Object.fromEntries(
    out.fields.map((f) => [f.name, { type: f.type, values: f.values }])
  )
}

describe("extractFields json", () => {
  it("adds one field per top-level key, numbers as numbers", () => {
    const out = run(
      ['{"level":"info","ms":12}', '{"level":"warn","ms":"7.5"}', "oops"],
      {
        format: "json",
      }
    )
    expect(out.level).toEqual({
      type: "string",
      values: ["info", "warn", null],
    })
    expect(out.ms).toEqual({ type: "number", values: [12, 7.5, null] })
    expect(Object.keys(out)).toEqual(["Time", "line", "level", "ms"])
  })

  it("reads paths with dots and indexes under their alias", () => {
    const out = run(['{"a":{"b":[{"c":1},{"c":2}]},"s":"x"}'], {
      format: "json",
      paths: [
        { path: "a.b[1].c", alias: "second" },
        { path: "s" },
        { path: "a.missing" },
        { path: "constructor" },
      ],
    })
    expect(out.second.values).toEqual([2])
    expect(out.s.values).toEqual(["x"])
    expect(out["a.missing"].values).toEqual([null])
    expect(
      Object.entries(out).find(([name]) => name === "constructor")?.[1].values
    ).toEqual([null])
  })

  it("writes nested values as JSON text", () => {
    expect(
      run(['{"o":{"k":1},"l":[1,2]}'], { format: "json" }).o.values
    ).toEqual(['{"k":1}'])
  })

  it("does not parse arrays or other JSON as rows", () => {
    expect(Object.keys(run(["[1,2]", "5"], { format: "json" }))).toEqual([
      "Time",
      "line",
    ])
  })
})

describe("extractFields kvp, delimiter and regexp", () => {
  it("reads key=value pairs with quoted values and key: value lists", () => {
    const out = run(['a=1 b="x y" c=z', "a: 2, b: q"], { format: "kvp" })
    expect(out.a).toEqual({ type: "number", values: [1, 2] })
    expect(out.b.values).toEqual(["x y", "q"])
    expect(out.c.values).toEqual(["z", null])
  })

  it("splits by the delimiter into fields named 1, 2, ...", () => {
    const out = run(["a;1", "b;2;extra"], {
      format: "delimiter",
      delimiter: ";",
    })
    expect(out["1"].values).toEqual(["a", "b"])
    expect(out["2"]).toEqual({ type: "number", values: [1, 2] })
    expect(out["3"].values).toEqual([null, "extra"])
  })

  it("turns named groups into fields", () => {
    const out = run(["GET /a 200", "POST /b 500", "junk"], {
      format: "regexp",
      regex: "(?<method>\\w+) (?<path>\\S+) (?<status>\\d+)",
    })
    expect(out.method.values).toEqual(["GET", "POST", null])
    expect(out.status).toEqual({ type: "number", values: [200, 500, null] })
  })

  it("adds nothing for a missing or invalid regexp", () => {
    for (const regex of [undefined, "("]) {
      expect(Object.keys(run(["a"], { format: "regexp", regex }))).toEqual([
        "Time",
        "line",
      ])
    }
  })
})

describe("extractFields auto and options", () => {
  it("tries JSON first and falls back to key=value", () => {
    const out = run(['{"a":1}', "b=2"])
    expect(out.a.values).toEqual([1, null])
    expect(out.b.values).toEqual([null, 2])
  })

  it("replaces every field but keeps time when asked", () => {
    expect(Object.keys(run(['{"a":1}'], { replace: true }))).toEqual(["a"])
    expect(
      Object.keys(run(['{"a":1}'], { replace: true, keepTime: true }))
    ).toEqual(["Time", "a"])
  })

  it("does not overwrite an existing field of the same name", () => {
    const out = run(['{"Time":"t","a":1}'], { format: "json" })
    expect(out.Time).toEqual({ type: "time", values: [0] })
    expect(out.a.values).toEqual([1])
  })

  it("leaves the frame alone when the source field is missing", () => {
    const frame = frameOf(['{"a":1}'])
    const [out] = extractFields([frame], {
      type: "extractFields",
      field: "nope",
    })
    expect(out).toEqual(frame)
  })

  it("skips rows that are not text", () => {
    const out = run([null, 5, '{"a":1}'], { format: "json" })
    expect(out.a.values).toEqual([null, null, 1])
  })
})
