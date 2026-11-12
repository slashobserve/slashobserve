import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook } from "@testing-library/react"
import { createElement, type ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { dashboardKeys, proposalKeys } from "@/api/keys"
import type { Dashboard } from "@/spec"

import { asDashboardSpec, useAddVersion } from "./queries"

const invalid = "Server returned an invalid Spec"

describe("asDashboardSpec", () => {
  it("returns a Spec with the dashboard shape as the same reference", () => {
    const spec = { specVersion: "0", title: "Checkout", panels: [] }
    expect(asDashboardSpec(spec)).toBe(spec)
  })

  it.each([
    ["null", null],
    ["a string", "dashboard"],
    [
      "another spec version",
      { specVersion: "1", title: "Checkout", panels: [] },
    ],
    ["missing panels", { specVersion: "0", title: "Checkout" }],
    ["non-array panels", { specVersion: "0", title: "Checkout", panels: {} }],
  ])("throws for %s", (_, raw) => {
    expect(() => asDashboardSpec(raw)).toThrow(invalid)
  })
})

const spec: Dashboard = {
  specVersion: "0",
  title: "Checkout",
  time: { from: "now-1h", to: "now" },
  panels: [],
}

function saved(version: number, extra: object = {}) {
  return {
    dashboard: { id: "d1" },
    version: { version, spec },
    ...extra,
  }
}

function stubSave(status: number, body: unknown) {
  const sent: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      sent.push(await request.json())
      return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return sent
}

function save(propose: boolean) {
  const client = new QueryClient()
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
  const hook = renderHook(() => useAddVersion("d1"), { wrapper })
  const run = () =>
    act(() =>
      hook.result.current.mutateAsync({
        spec,
        summary: "Edit",
        parent_version: 2,
        propose,
      })
    )
  return { client, run }
}

afterEach(() => vi.unstubAllGlobals())

describe("useAddVersion", () => {
  it("asks the server to merge a stale save and stores a saved version", async () => {
    const sent = stubSave(201, saved(3))
    const { client, run } = save(false)
    const outcome = await run()
    expect(sent).toEqual([
      { spec, summary: "Edit", parent_version: 2, merge: "auto" },
    ])
    expect(outcome).toMatchObject({
      kind: "saved",
      version: 3,
      mergedFrom: null,
    })
    expect(
      client.getQueryData<{ version: { version: number } }>(
        dashboardKeys.latest("d1")
      )?.version.version
    ).toBe(3)
  })

  it("reports the versions a merged save came from", async () => {
    stubSave(
      201,
      saved(5, { merged_from: { base_version: 2, onto_version: 4 } })
    )
    const { run } = save(false)
    expect(await run()).toMatchObject({
      kind: "saved",
      version: 5,
      mergedFrom: { base_version: 2, onto_version: 4 },
    })
  })

  it("asks to propose only when the caller may only comment", async () => {
    const sent = stubSave(202, {
      proposal_id: "p1",
      revision: 1,
      opened: true,
    })
    await save(true).run()
    expect(sent[0]).toMatchObject({ merge: "auto", propose_if_needed: true })
  })

  it("answers a proposal without storing a version and refreshes the open proposals", async () => {
    stubSave(202, { proposal_id: "p1", revision: 2, opened: false })
    const { client, run } = save(true)
    client.setQueryData(proposalKeys.list("d1", "open"), { proposals: [] })
    const outcome = await run()
    expect(outcome).toEqual({
      kind: "proposed",
      proposalId: "p1",
      revision: 2,
      opened: false,
    })
    expect(client.getQueryData(dashboardKeys.latest("d1"))).toBeUndefined()
    expect(
      client.getQueryState(proposalKeys.list("d1", "open"))?.isInvalidated
    ).toBe(true)
  })

  it("throws the server's conflict with the overlapping units", async () => {
    stubSave(409, {
      error: "stale",
      code: "version_conflict",
      latest_version: 4,
      conflicts: [
        { kind: "panel", id: "p", reason: "both_changed", message: "Both" },
      ],
    })
    await expect(save(false).run()).rejects.toMatchObject({
      status: 409,
      latestVersion: 4,
      conflicts: [{ id: "p" }],
    })
  })
})
