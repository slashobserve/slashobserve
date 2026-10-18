import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { panelRetry } from "./use-panel-data"

const error = (status: number, code: string) =>
  new ApiError(status, code, "failed")

describe("panelRetry", () => {
  it("never retries a query timeout", () => {
    expect(panelRetry(0, error(504, "query_timeout"))).toBe(false)
  })

  it("retries an unavailable datasource once", () => {
    const unavailable = error(502, "datasource_unavailable")
    expect(panelRetry(0, unavailable)).toBe(true)
    expect(panelRetry(1, unavailable)).toBe(false)
  })

  it("stops other retryable errors at three retries", () => {
    const busy = error(503, "busy")
    expect(panelRetry(2, busy)).toBe(true)
    expect(panelRetry(3, busy)).toBe(false)
  })

  it("never retries a client error", () => {
    expect(panelRetry(0, error(404, "not_found"))).toBe(false)
  })
})
