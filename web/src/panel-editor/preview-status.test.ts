import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import {
  previewBlocksSave,
  previewErrorText,
  previewErrorShownInline,
  previewStatusText,
  seriesCount,
  type PreviewStatus,
} from "./preview-status"

const now = Date.parse("2026-10-19T12:00:10Z")

describe("seriesCount", () => {
  it("counts number fields across frames", () => {
    expect(
      seriesCount([
        {
          fields: [
            { name: "Time", type: "time", values: [1] },
            { name: "a", type: "number", values: [1] },
            { name: "b", type: "number", values: [1] },
          ],
        },
        {
          fields: [
            { name: "Time", type: "time", values: [1] },
            { name: "c", type: "number", values: [1] },
            { name: "d", type: "string", values: ["x"] },
          ],
        },
      ])
    ).toBe(3)
  })

  it("counts nothing for no frames", () => {
    expect(seriesCount([])).toBe(0)
  })
})

describe("previewErrorShownInline", () => {
  const invalid = new ApiError(
    422,
    "query_invalid",
    "bad",
    [],
    undefined,
    undefined,
    "A"
  )

  it("is true when the query's own lint error already names the problem", () => {
    expect(previewErrorShownInline(invalid, new Set(["A"]))).toBe(true)
  })

  it("is false when no lint error is shown for that query", () => {
    expect(previewErrorShownInline(invalid, new Set(["B"]))).toBe(false)
    expect(previewErrorShownInline(invalid, new Set())).toBe(false)
  })

  it("is false for faults the lint does not find", () => {
    for (const code of ["expression_error", "unknown_macro", "query_timeout"]) {
      const error = new ApiError(
        422,
        code,
        "bad",
        [],
        undefined,
        undefined,
        "A"
      )
      expect(previewErrorShownInline(error, new Set(["A"]))).toBe(false)
    }
    expect(previewErrorShownInline(new NetworkError("x"), new Set(["A"]))).toBe(
      false
    )
  })

  it("is false when the error names no query", () => {
    expect(
      previewErrorShownInline(
        new ApiError(422, "query_invalid", "bad"),
        new Set(["A"])
      )
    ).toBe(false)
  })
})

describe("previewBlocksSave", () => {
  it("blocks a save only for a fault in the draft itself", () => {
    expect(previewBlocksSave(new ApiError(422, "query_invalid", "bad"))).toBe(
      true
    )
    expect(previewBlocksSave(new ApiError(422, "invalid_spec", "bad"))).toBe(
      true
    )
    for (const code of [
      "query_empty",
      "query_timeout",
      "datasource_unavailable",
      "too_many_requests",
      "internal",
    ]) {
      expect(previewBlocksSave(new ApiError(504, code, "slow"))).toBe(false)
    }
    expect(previewBlocksSave(new NetworkError("offline"))).toBe(false)
  })
})

describe("previewErrorText", () => {
  it("names the first issue of an invalid spec", () => {
    const error = new ApiError(422, "invalid_spec", "invalid", [
      {
        path: "/queries/0/query",
        code: "minLength",
        message: "Query is empty",
      },
    ])
    expect(previewErrorText(error)).toBe(
      "The draft is not valid: Query is empty"
    )
  })

  it("handles an invalid spec without issues", () => {
    expect(previewErrorText(new ApiError(422, "invalid_spec", "invalid"))).toBe(
      "The draft is not valid."
    )
  })

  const rows: [number, string, string][] = [
    [
      429,
      "too_many_requests",
      "Too many requests right now. The preview runs again in a moment.",
    ],
    [502, "datasource_unavailable", "The datasource is not answering."],
    [504, "query_timeout", "The draft took too long to run."],
  ]
  it.each(rows)("explains %i %s", (status, code, text) => {
    expect(previewErrorText(new ApiError(status, code, "raw"))).toBe(text)
  })

  it("passes any other API message through", () => {
    expect(
      previewErrorText(new ApiError(422, "query_invalid", "parse error at 1:5"))
    ).toBe("parse error at 1:5")
  })

  const withRef = (code: string, message: string) =>
    new ApiError(502, code, message, [], undefined, undefined, "B")

  it("names the failed query on a passed-through message", () => {
    expect(
      previewErrorText(withRef("query_invalid", "Parse error at 1:5"))
    ).toBe("Query B failed: parse error at 1:5")
  })

  it("names the failed query on a timeout and an unavailable datasource", () => {
    expect(previewErrorText(withRef("query_timeout", "raw"))).toBe(
      "Query B failed: it took too long to run."
    )
    expect(previewErrorText(withRef("datasource_unavailable", "raw"))).toBe(
      "Query B failed: the datasource is not answering."
    )
  })

  it("falls back for errors that are not API errors", () => {
    expect(previewErrorText(new NetworkError("offline"))).toBe(
      "The preview could not run."
    )
    expect(previewErrorText(undefined)).toBe("The preview could not run.")
  })
})

describe("previewStatusText", () => {
  const ran = (executedAt: string, series: number): PreviewStatus => ({
    kind: "ok",
    executedAt,
    series,
  })

  it("matches the canvas sentence", () => {
    expect(previewStatusText(ran("2026-10-19T12:00:05Z", 3), now)).toBe(
      "Draft ran 5 s ago, 3 series"
    )
  })

  it("lower-cases Just now", () => {
    expect(previewStatusText(ran("2026-10-19T12:00:09Z", 1), now)).toBe(
      "Draft ran just now, 1 series"
    )
  })

  it("reads the other states", () => {
    expect(previewStatusText({ kind: "invalid" }, now)).toBe(
      "The draft is not valid yet."
    )
    expect(previewStatusText({ kind: "waiting" }, now)).toBe(
      "Waiting for variables."
    )
    expect(previewStatusText({ kind: "running" }, now)).toBe(
      "Running the draft."
    )
    expect(previewStatusText({ kind: "error", text: "Nope." }, now)).toBe(
      "The draft did not run."
    )
  })
})
