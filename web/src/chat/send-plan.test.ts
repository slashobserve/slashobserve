import { describe, expect, it } from "vitest"

import { planSend } from "./send-plan"

const dashboardId = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"

describe("planSend", () => {
  it("opens a new build for a plain prompt without a dashboard", () => {
    expect(planSend({ command: null, text: "checkout health" })).toEqual({
      input: { prompt: "checkout health" },
      opens: "build",
    })
  })

  it("revises in place for a plain prompt with a dashboard", () => {
    expect(planSend({ command: null, text: "add p99", dashboardId })).toEqual({
      input: { prompt: "add p99", dashboardId },
      opens: "same",
    })
  })

  it("imports into a new build even beside a dashboard", () => {
    expect(
      planSend({ command: "import", text: "https://grafana/d/x", dashboardId })
    ).toEqual({ input: { prompt: "https://grafana/d/x" }, opens: "build" })
  })

  it("revises the open dashboard", () => {
    expect(
      planSend({ command: "revise", text: "add p99", dashboardId })
    ).toEqual({ input: { prompt: "add p99", dashboardId }, opens: "same" })
  })

  it("refuses to revise without a dashboard", () => {
    expect(planSend({ command: "revise", text: "add p99" })).toEqual({
      error: "Open a dashboard to revise it.",
    })
  })

  it("investigates a dashboard panel over a window and stays in place", () => {
    expect(
      planSend({
        command: "investigate",
        text: "why did p95 jump",
        dashboardId,
        panelId: "latency",
        from: "now-6h",
        to: "now",
      })
    ).toEqual({
      input: {
        prompt: "why did p95 jump",
        kind: "investigate",
        dashboardId,
        panelId: "latency",
        from: "now-6h",
        to: "now",
      },
      opens: "same",
    })
  })

  it("opens the chat for an investigation without a dashboard", () => {
    expect(planSend({ command: "investigate", text: "why" })).toEqual({
      input: { prompt: "why", kind: "investigate" },
      opens: "chat",
    })
  })

  it("drops a panel without a dashboard and a window without both ends", () => {
    expect(
      planSend({
        command: "investigate",
        text: "why",
        panelId: "latency",
        from: "now-6h",
      })
    ).toEqual({
      input: { prompt: "why", kind: "investigate" },
      opens: "chat",
    })
  })
})
