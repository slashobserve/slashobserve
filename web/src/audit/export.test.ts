import { describe, expect, test } from "vitest"

import {
  auditExportUrl,
  downloadStartedText,
  exportLimitText,
  exportRowLimit,
} from "./export"

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

describe("exportLimitText", () => {
  test("says nothing up to the limit", () => {
    expect(exportLimitText(0)).toBe(null)
    expect(exportLimitText(exportRowLimit)).toBe(null)
  })

  test("names the match count past the limit", () => {
    expect(exportLimitText(exportRowLimit + 1)).toBe(
      "1,000,001 events match; the export stops at 1,000,000."
    )
  })
})

describe("downloadStartedText", () => {
  test("names the format that was chosen", () => {
    expect(downloadStartedText("csv")).toBe(
      "Download started: the audit log as CSV."
    )
    expect(downloadStartedText("ndjson")).toBe(
      "Download started: the audit log as NDJSON."
    )
  })
})
