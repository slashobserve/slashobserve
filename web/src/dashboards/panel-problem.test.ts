import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { panelProblem } from "./panel-problem"
import { VariableLoadError } from "./variables"

describe("panelProblem", () => {
  it("says there is no data in the range when a variable has no values, without Retry", () => {
    const error = new ApiError(
      422,
      "variable_empty",
      'variable has no values: "instance"',
      [],
      undefined,
      undefined,
      "A"
    )
    const problem = panelProblem(error)
    expect(problem.title).toBe("No data in this range")
    expect(problem.message).toContain('"instance"')
    expect(problem.retry).toBe(false)
    expect(problem.empty).toBe(true)
  })

  it("does not offer Retry for a request the server refused as invalid", () => {
    const error = new ApiError(
      400,
      "invalid_request",
      "the time range must be at most 400 days"
    )
    expect(panelProblem(error)).toEqual({
      title: "This request cannot run",
      message: "the time range must be at most 400 days",
      retry: false,
      empty: false,
    })
  })

  it("names a failed query and offers Retry for a datasource error", () => {
    const error = new ApiError(
      502,
      "datasource_unavailable",
      "Prometheus did not answer",
      [],
      undefined,
      undefined,
      "B"
    )
    expect(panelProblem(error)).toEqual({
      title: "Datasource unavailable",
      message: "Query B failed: prometheus did not answer",
      retry: true,
      empty: false,
    })
  })

  it.each([
    ["time_range_in_future", "Time range starts in the future"],
    ["time_range_too_long", "Time range too long"],
  ])("titles a %s refusal", (code, title) => {
    const error = new ApiError(422, code, "refused", [])
    expect(panelProblem(error).title).toBe(title)
  })

  it("offers Retry when a variable's values could not be loaded", () => {
    const problem = panelProblem(new VariableLoadError("Host"))
    expect(problem.title).toBe("Variable failed to load")
    expect(problem.retry).toBe(true)
  })

  it("falls back for an unknown failure", () => {
    expect(panelProblem(new Error("boom"))).toEqual({
      title: "Panel failed to load",
      message: "boom",
      retry: true,
      empty: false,
    })
  })
})
