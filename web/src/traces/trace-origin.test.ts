import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { describe, expect, it } from "vitest"

import {
  currentTraceOrigin,
  followTraceOrigin,
  nextTraceOrigin,
  traceOriginOf,
} from "./trace-origin"

const logs = { pathname: "/logs/ch", href: "/logs/ch?q=error&from=now-1h" }
const traces = { pathname: "/traces/tempo", href: "/traces/tempo?service=pay" }
const trace = { pathname: "/t/" + "a".repeat(32), href: "/t/" + "a".repeat(32) }
const dashboard = { pathname: "/d/abc", href: "/d/abc" }

describe("traceOriginOf", () => {
  it("names the Logs page and keeps its whole address", () => {
    expect(traceOriginOf(logs)).toEqual({
      label: "Back to logs",
      href: "/logs/ch?q=error&from=now-1h",
    })
  })

  it("names the Traces page", () => {
    expect(traceOriginOf(traces)?.label).toBe("Back to traces")
  })

  it("offers nothing for a page that is neither", () => {
    expect(traceOriginOf(dashboard)).toBeNull()
  })
})

describe("nextTraceOrigin", () => {
  const kept = { label: "Back to logs", href: logs.href }

  it("remembers the page a trace was opened from", () => {
    expect(nextTraceOrigin(null, logs, trace)?.href).toBe(logs.href)
  })

  it("keeps the origin when the next trace opens from a trace", () => {
    expect(nextTraceOrigin(kept, trace, trace)).toBe(kept)
  })

  it("forgets the origin when the trace opens from somewhere else", () => {
    expect(nextTraceOrigin(kept, dashboard, trace)).toBeNull()
  })

  it("ignores navigation that does not land on a trace", () => {
    expect(nextTraceOrigin(kept, trace, dashboard)).toBe(kept)
  })

  it("keeps the origin when there is no previous page", () => {
    expect(nextTraceOrigin(kept, undefined, trace)).toBe(kept)
  })
})

describe("followTraceOrigin", () => {
  it("records the Logs address when the router opens a trace", async () => {
    const root = createRootRoute()
    const tree = root.addChildren([
      createRoute({ getParentRoute: () => root, path: "/logs/$uid" }),
      createRoute({ getParentRoute: () => root, path: "/t/$id" }),
      createRoute({ getParentRoute: () => root, path: "/other" }),
    ])
    const router = createRouter({
      routeTree: tree,
      history: createMemoryHistory({ initialEntries: ["/logs/ch?q=error"] }),
    })
    await router.load()
    followTraceOrigin(router)

    await router.navigate({ href: "/t/" + "b".repeat(32) })
    expect(currentTraceOrigin()).toEqual({
      label: "Back to logs",
      href: "/logs/ch?q=error",
    })

    await router.navigate({ href: "/other" })
    await router.navigate({ href: "/t/" + "b".repeat(32) })
    expect(currentTraceOrigin()).toBeNull()
  })
})
