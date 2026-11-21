import { describe, expect, test } from "vitest"

import { auditExportUrl } from "./export"

describe("auditExportUrl", () => {
  test("sends the query, the window and the format", () => {
    const url = new URL(
      auditExportUrl(
        {
          q: "actor:Zoë",
          since: "2026-11-20T06:00:00.000Z",
          until: "2026-11-21T06:00:00.000Z",
        },
        "csv"
      ),
      "http://localhost"
    )
    expect(url.pathname).toBe("/api/v1/audit/export")
    expect(Object.fromEntries(url.searchParams)).toEqual({
      q: "actor:Zoë",
      since: "2026-11-20T06:00:00.000Z",
      until: "2026-11-21T06:00:00.000Z",
      format: "csv",
    })
  })

  test("leaves out an empty query", () => {
    const url = auditExportUrl(
      {
        q: "",
        since: "2026-11-20T06:00:00.000Z",
        until: "2026-11-21T06:00:00.000Z",
      },
      "ndjson"
    )
    expect(url).not.toContain("q=")
  })
})
