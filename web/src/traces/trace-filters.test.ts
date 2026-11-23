import { describe, expect, it } from "vitest"

import { queryLabel, traceFilterChips } from "./trace-filters"

describe("traceFilterChips", () => {
  it("gives no chip for an empty search", () => {
    expect(traceFilterChips("tempo", {})).toEqual([])
  })

  it("ignores keys that are not filters", () => {
    expect(
      traceFilterChips("tempo", {
        ask: "slow checkout",
        from: "now-1h",
        to: "now",
      })
    ).toEqual([])
  })

  it("gives chips for service and min duration in order, with the form's labels", () => {
    expect(
      traceFilterChips("tempo", { min: "500ms", service: "frontend" })
    ).toEqual([
      { key: "service", label: "Service", value: "frontend" },
      { key: "min", label: "Min span duration", value: "500ms" },
    ])
  })

  it("shows errors as its label alone", () => {
    expect(traceFilterChips("tempo", { errors: "1" })).toEqual([
      { key: "errors", label: "Errors only", value: "" },
    ])
  })

  it("gives one chip for a TraceQL query alone", () => {
    expect(traceFilterChips("tempo", { q: "{ duration > 1s }" })).toEqual([
      { key: "q", label: "TraceQL", value: "{ duration > 1s }" },
    ])
  })

  it("gives all six chips in order", () => {
    expect(
      traceFilterChips("tempo", {
        q: "{ }",
        errors: "1",
        max: "2s",
        min: "250ms",
        span: "GET /cart",
        service: "cart",
      }).map((chip) => [chip.key, chip.label, chip.value])
    ).toEqual([
      ["service", "Service", "cart"],
      ["span", "Span name", "GET /cart"],
      ["min", "Min span duration", "250ms"],
      ["max", "Max span duration", "2s"],
      ["errors", "Errors only", ""],
      ["q", "TraceQL", "{ }"],
    ])
  })

  it("labels a written query by the datasource's language", () => {
    expect(
      traceFilterChips("jaeger", { q: "service=cart" }).map(
        (chip) => chip.label
      )
    ).toEqual(["Search"])
    expect(queryLabel("tempo")).toBe("TraceQL")
  })
})
