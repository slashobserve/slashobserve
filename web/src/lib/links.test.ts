import { describe, expect, it } from "vitest"

import {
  buildPath,
  chatPath,
  dashboardLink,
  dashboardPath,
  folderPath,
  logsUrl,
  slugOf,
  traceLink,
  traceUrl,
} from "./links"

describe("slugOf", () => {
  it.each([
    ["Checkout health", "checkout-health"],
    ["Thanos / Query -- p99 (5m)", "thanos-query-p99-5m"],
    ["  --Shop!--  ", "shop"],
    ["K8s 1.30 nodes", "k8s-1-30-nodes"],
    ["Café Über ops", "caf-ber-ops"],
    ["監視ダッシュボード", ""],
    ["İstanbul", "stanbul"],
    ["Kube", "ube"],
    ["🔥 Errors 🔥", "errors"],
    ["", ""],
    ["!!! ---", ""],
    ["Versions", ""],
    ["Versions v2", "versions-v2"],
    ["a".repeat(70), "a".repeat(60)],
    ["a".repeat(59) + " bcd", "a".repeat(59)],
  ])("slugs %j as %j", (title, slug) => {
    expect(slugOf(title)).toBe(slug)
  })
})

describe("dashboard addresses", () => {
  const id = "0b6f9c1e-7a52-4c1b-9d3e-2f8a6b4c5d10"

  it.each([
    ["with a title", "Checkout health", `/d/${id}/checkout-health`],
    ["without a title", undefined, `/d/${id}`],
    ["whose title has no slug", "監視", `/d/${id}`],
  ])("builds the path %s", (_, title, path) => {
    expect(dashboardPath(id, title)).toBe(path)
  })

  it("escapes the id", () => {
    expect(dashboardPath("a/b")).toBe("/d/a%2Fb")
  })

  it("gives the router the slug only when the title has one", () => {
    expect(dashboardLink(id, "Shop").params).toEqual({
      dashboardId: id,
      slug: "shop",
    })
    expect(dashboardLink(id, "!!!").params.slug).toBeUndefined()
    expect(dashboardLink(id).params.slug).toBeUndefined()
  })

  it("builds the folder, chat and build paths", () => {
    expect(folderPath(id)).toBe(`/f/${id}`)
    expect(chatPath(id)).toBe(`/c/${id}`)
    expect(buildPath(id)).toBe(`/b/${id}`)
  })
})

describe("logs and trace addresses", () => {
  const traceId = "4bf92f3577b34da6a3ce929d0e0e4736"

  it("builds a trace address with only the datasource", () => {
    expect(traceUrl(traceId, "tempo")).toBe(`/t/${traceId}?ds=tempo`)
  })

  it("appends the range after the datasource", () => {
    expect(traceUrl(traceId, "ch", { from: "1760000000000", to: "now" })).toBe(
      `/t/${traceId}?ds=ch&from=1760000000000&to=now`
    )
  })

  it("gives the router the trace id and search", () => {
    expect(traceLink(traceId, "tempo")).toEqual({
      to: "/t/$traceId",
      params: { traceId },
      search: { ds: "tempo", from: undefined, to: undefined },
    })
  })

  it("percent-encodes the logs query", () => {
    expect(
      logsUrl({
        ds: "loki",
        q: '{service_name="checkout"}',
        from: "now-1h",
        to: "now",
      })
    ).toBe(
      "/logs?ds=loki&q=%7Bservice_name%3D%22checkout%22%7D&from=now-1h&to=now"
    )
  })
})
