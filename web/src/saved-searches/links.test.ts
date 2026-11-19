import { describe, expect, it } from "vitest"

import { savedSearchLocation } from "./links"

describe("savedSearchLocation", () => {
  it("reopens a logs search on the logs page with its columns", () => {
    expect(
      savedSearchLocation({
        id: "5f0c1e0e-0000-4000-8000-000000000001",
        kind: "logs",
        datasource_uid: "es",
        query: "level:error",
        filters: { columns: ["host", "message"] },
        from: "now-6h",
        to: "now",
      })
    ).toEqual({
      to: "/logs/$datasourceUid",
      params: { datasourceUid: "es" },
      search: {
        q: "level:error",
        cols: "host,message",
        from: "now-6h",
        to: "now",
        saved: "5f0c1e0e-0000-4000-8000-000000000001",
      },
    })
  })

  it("leaves cols out when a logs search has no columns", () => {
    const location = savedSearchLocation({
      id: "s1",
      kind: "logs",
      datasource_uid: "es",
      query: "x",
      filters: {},
      from: "now-1h",
      to: "now",
    })
    expect(location.search).not.toHaveProperty("cols", expect.anything())
  })

  it("reopens a trace search on the traces page with its filters", () => {
    expect(
      savedSearchLocation({
        id: "s1",
        kind: "traces",
        datasource_uid: "tempo",
        query: null,
        filters: {
          service: "checkout",
          span: "POST /pay",
          min: "250ms",
          max: "2s",
          errors: true,
        },
        from: "now-1h",
        to: "now",
      })
    ).toEqual({
      to: "/traces/$datasourceUid",
      params: { datasourceUid: "tempo" },
      search: {
        service: "checkout",
        span: "POST /pay",
        min: "250ms",
        max: "2s",
        errors: "1",
        from: "now-1h",
        to: "now",
      },
    })
  })

  it("keeps a Tempo query and drops errors when it is false", () => {
    const location = savedSearchLocation({
      id: "s1",
      kind: "traces",
      datasource_uid: "tempo",
      query: "{ status = error }",
      filters: { errors: false },
      from: "now-1h",
      to: "now",
    })
    expect(location.search).toMatchObject({ q: "{ status = error }" })
    expect(location.search).not.toHaveProperty("errors", expect.anything())
  })

  it("reopens a metrics search on the metrics page, as a table when saved as one", () => {
    expect(
      savedSearchLocation({
        id: "s2",
        kind: "metrics",
        datasource_uid: "prom",
        query: "up",
        filters: { mode: "table" },
        from: "now-6h",
        to: "now",
      })
    ).toEqual({
      to: "/explore/metrics/$datasourceUid",
      params: { datasourceUid: "prom" },
      search: { q: "up", mode: "table", from: "now-6h", to: "now" },
    })
  })

  it("leaves the mode out of a metrics graph", () => {
    const location = savedSearchLocation({
      id: "s3",
      kind: "metrics",
      datasource_uid: "prom",
      query: "up",
      filters: {},
      from: "now-1h",
      to: "now",
    })
    expect(location.search).toHaveProperty("mode", undefined)
  })
})
