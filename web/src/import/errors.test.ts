import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import {
  DatasourcesUnresolvedError,
  importFailure,
  previewFailure,
  toImportError,
} from "./errors"

function api(status: number, code: string, message = "server text") {
  return new ApiError(status, code, message)
}

const tooLarge = "This file is larger than 8 MiB, the import limit."
const generic = "Could not read this dashboard. Try again."

describe("previewFailure", () => {
  it("explains a body that is too large", () => {
    expect(previewFailure(api(413, "too_large"))).toEqual({
      message: tooLarge,
      retry: false,
    })
  })

  it.each([
    "invalid_json",
    "not_a_dashboard",
    "unsupported_format",
    "too_many_panels",
  ])("shows the server text for %s", (code) => {
    expect(previewFailure(api(422, code, "Not a dashboard."))).toEqual({
      message: "Not a dashboard.",
      retry: false,
    })
  })

  it("shows the server text for a bad mapping and asks to reset it", () => {
    expect(previewFailure(api(422, "invalid_mapping", "Bad mapping."))).toEqual(
      { message: "Bad mapping.", retry: false, resetMapping: true }
    )
  })

  it("offers a retry for a server error and a network error", () => {
    expect(previewFailure(api(500, "internal"))).toEqual({
      message: generic,
      retry: true,
    })
    expect(previewFailure(new TypeError("Failed to fetch"))).toEqual({
      message: generic,
      retry: true,
    })
  })
})

describe("importFailure", () => {
  it("explains a busy importer", () => {
    expect(importFailure(api(429, "too_many_imports"))).toEqual({
      message: "Another import is still running. Try again in a moment.",
      retry: false,
    })
  })

  it("asks for datasources and carries the rows from the response", () => {
    const rows = [
      {
        key: "p",
        name: null,
        type: "prometheus",
        kind: "prometheus",
        panels: 1,
        match: null,
        matched_by: null,
        candidates: ["prom"],
      },
    ]
    const error = new DatasourcesUnresolvedError(
      api(422, "datasources_unresolved"),
      rows
    )
    expect(importFailure(error)).toEqual({
      message: "Choose a datasource for each row first.",
      retry: false,
      datasources: rows,
    })
  })

  it("quotes the first issue of an invalid spec", () => {
    const error = new ApiError(422, "invalid_spec", "invalid", [
      { path: "/panels/0", code: "x", message: "contains a NUL character." },
    ])
    expect(importFailure(error).message).toBe(
      "The converted dashboard could not be saved: contains a NUL character."
    )
  })

  it("handles an invalid spec with no issues", () => {
    expect(importFailure(api(422, "invalid_spec")).message).toBe(
      "The converted dashboard could not be saved."
    )
  })

  it("explains a body that is too large", () => {
    expect(importFailure(api(413, "too_large")).message).toBe(tooLarge)
  })

  it("falls back for anything else", () => {
    expect(importFailure(api(500, "internal")).message).toBe(
      "Could not import. Try again."
    )
    expect(importFailure(new Error("boom")).message).toBe(
      "Could not import. Try again."
    )
  })
})

describe("toImportError", () => {
  it("keeps the datasources of an unresolved import", () => {
    const body = {
      error: "unresolved",
      code: "datasources_unresolved",
      datasources: [{ key: "p" }],
    }
    const error = toImportError(new Response(null, { status: 422 }), body)
    expect(error).toBeInstanceOf(DatasourcesUnresolvedError)
    expect((error as DatasourcesUnresolvedError).datasources).toEqual([
      { key: "p" },
    ])
  })

  it("returns a plain ApiError for other responses", () => {
    const error = toImportError(new Response(null, { status: 429 }), {
      error: "busy",
      code: "too_many_imports",
    })
    expect(error).not.toBeInstanceOf(DatasourcesUnresolvedError)
    expect(error.code).toBe("too_many_imports")
  })
})
