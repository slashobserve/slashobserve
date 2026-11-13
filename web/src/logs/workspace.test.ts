import { describe, expect, it } from "vitest"

import { logsPageParams } from "./workspace"

const range = { from: "now-1h", to: "now" }

describe("logsPageParams", () => {
  it("asks the first page by its size with no row limit", () => {
    const params = logsPageParams({
      query: '{service_name="checkout"}',
      range,
      pageSize: 50,
      page: 1,
      cursor: undefined,
    })
    expect(params).toEqual({
      query: '{service_name="checkout"}',
      kind: "instant",
      from: "now-1h",
      to: "now",
      pageSize: 50,
      page: 1,
    })
    expect(params).not.toHaveProperty("limit")
    expect(params).not.toHaveProperty("cursor")
  })

  it("carries the remembered cursor of a Loki page 2 and no row limit", () => {
    const params = logsPageParams({
      query: '{service_name="checkout"}',
      range,
      pageSize: 100,
      page: 2,
      cursor: "c3RhcnQ",
    })
    expect(params).toMatchObject({ pageSize: 100, page: 2, cursor: "c3RhcnQ" })
    expect(params).not.toHaveProperty("limit")
  })

  it("asks a ClickHouse page by number alone", () => {
    expect(
      logsPageParams({
        query: "SELECT 1",
        range,
        pageSize: 250,
        page: 7,
        cursor: undefined,
      })
    ).toMatchObject({ pageSize: 250, page: 7 })
  })
})
