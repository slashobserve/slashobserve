import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import { renderWithProviders } from "@/test/render"

import { QueryFailure } from "./query-failure"

function answerLint(diagnostics: unknown[]) {
  const fetch = vi.fn(async () => {
    return new Response(JSON.stringify({ diagnostics }), {
      status: 200,
      headers: { "content-type": "application/json" },
    })
  })
  vi.stubGlobal("fetch", fetch)
  return fetch
}

const rejected = new ApiError(
  422,
  "query_invalid",
  "invalid query: 1:7: parse error"
)

function show(error: unknown, query = "sum(up") {
  return renderWithProviders(
    <QueryFailure uid="prom" query={query} error={error} onRetry={vi.fn()} />,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("QueryFailure", () => {
  it("shows the line, column and caret of a query the server rejected", async () => {
    answerLint([
      {
        severity: "error",
        message: "unclosed left parenthesis",
        start: 3,
        end: 6,
      },
    ])
    show(rejected)
    expect(
      await screen.findByText("Line 1, column 4: unclosed left parenthesis", {
        exact: false,
      })
    ).toBeInTheDocument()
    expect(screen.getByText(/Nothing ran/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull()
  })

  it("keeps the server's message and Retry when the lint finds no error", async () => {
    answerLint([])
    show(rejected)
    expect(await screen.findByRole("button", { name: "Retry" })).toBeVisible()
    expect(screen.getByText(/parse error/)).toBeInTheDocument()
  })

  it("does not lint a failure that is not a rejected query", async () => {
    const fetch = answerLint([])
    show(new ApiError(502, "datasource_unavailable", "Datasource unavailable"))
    expect(await screen.findByRole("button", { name: "Retry" })).toBeVisible()
    expect(fetch).not.toHaveBeenCalled()
  })
})
