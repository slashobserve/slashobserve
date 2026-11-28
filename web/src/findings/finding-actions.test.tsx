import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { MakeDashboardAction, PinAction } from "./finding-actions"
import type { Finding } from "./queries"
import { must } from "@/test/must"

function finding(patch: Partial<Finding> = {}): Finding {
  return {
    id: "f1",
    observation_id: "o9",
    agent_request_id: "r1",
    dashboard_id: null,
    folder_id: null,
    ask_on: null,
    restricted: false,
    title: "Checkout errors rose after the deploy",
    markdown: "The error rate doubled at 10:02.",
    evidence: [
      { id: "evidence-1", type: "timeseries", title: "Error rate" },
    ] as unknown as Finding["evidence"],
    from: "2026-11-06T10:00:00Z",
    to: "2026-11-06T11:00:00Z",
    as_of: null,
    annotation_id: null,
    created_by: { kind: "user", id: "u1", name: "Ada" },
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: "r1",
      label: "Observe Agent for Ada",
    },
    created_at: "2026-11-06T11:05:00Z",
    url: "",
    suggested_change: null,
    observe_worthy: false,
    ...patch,
  }
}

const route = { route: { path: "/", url: "/" } }

describe("MakeDashboardAction", () => {
  it("opens a made dashboard with its chat in the side panel", async () => {
    const dashboardId = "33333333-3333-4333-8333-333333333333"
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              dashboard: { id: dashboardId, title: "Checkout errors" },
            }),
            { status: 201, headers: { "Content-Type": "application/json" } }
          )
      )
    )
    const { user, router } = renderWithProviders(
      <MakeDashboardAction finding={finding()} chatId="c1" />,
      route
    )
    await user.click(
      await screen.findByRole("button", { name: "Make a dashboard" })
    )
    await vi.waitFor(() =>
      expect(router?.state.location.pathname).toBe(
        `/d/${dashboardId}/checkout-errors`
      )
    )
    expect(router?.state.location.search).toEqual({ tab: "chat", chat: "c1" })
    vi.unstubAllGlobals()
  })

  it.each([
    ["moves the chat into the side panel with a view transition", "c1", true],
    ["opens a dashboard with no chat to move without one", undefined, false],
  ])("%s", async (_name, chatId, viewTransition) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              dashboard: {
                id: "33333333-3333-4333-8333-333333333333",
                title: "A",
              },
            }),
            { status: 201, headers: { "Content-Type": "application/json" } }
          )
      )
    )
    const { user, router } = renderWithProviders(
      <MakeDashboardAction finding={finding()} chatId={chatId} />,
      route
    )
    const navigate = vi.spyOn(router!, "navigate")
    await user.click(
      await screen.findByRole("button", { name: "Make a dashboard" })
    )
    await vi.waitFor(() => expect(navigate).toHaveBeenCalledOnce())
    expect(must(navigate.mock.calls[0])[0]).toMatchObject({ viewTransition })
    vi.unstubAllGlobals()
  })
})

type Pinning = {
  dashboard_id: string | null
  pinned: boolean
  can_pin: boolean
}

function stubObservation(pinning: Pinning) {
  const calls: { method: string; path: string; body: unknown }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const path = new URL(input.url).pathname
      const body = input.method === "GET" ? undefined : await input.json()
      calls.push({ method: input.method, path, body })
      const json =
        input.method === "GET"
          ? { observation: { id: "o9", ...pinning }, folder: null }
          : { dashboard_id: pinning.dashboard_id, annotation_id: "a1" }
      return new Response(JSON.stringify(json), {
        status: input.method === "GET" ? 200 : 201,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return calls
}

describe("PinAction", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("pins the finding's observation to the dashboard its run was about", async () => {
    const calls = stubObservation({
      dashboard_id: "d1",
      pinned: false,
      can_pin: true,
    })
    const { user } = renderWithProviders(
      <PinAction finding={finding({ dashboard_id: "d1" })} />,
      route
    )
    await user.click(
      await screen.findByRole("button", { name: "Pin to dashboard" })
    )
    expect(await screen.findByText("Pinned to the dashboard")).toBeVisible()
    expect(calls).toContainEqual({
      method: "POST",
      path: "/api/v1/observations/o9/pin",
      body: {},
    })
  })

  it("offers Pin to dashboard only when the observation says the caller may pin", async () => {
    stubObservation({ dashboard_id: "d1", pinned: false, can_pin: false })
    renderWithProviders(
      <PinAction finding={finding({ dashboard_id: "d1" })} />,
      route
    )
    await vi.waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(
      screen.queryByRole("button", { name: "Pin to dashboard" })
    ).not.toBeInTheDocument()
  })

  it("still shows a pinned observation to someone who cannot pin", async () => {
    stubObservation({ dashboard_id: "d1", pinned: true, can_pin: false })
    renderWithProviders(
      <PinAction finding={finding({ dashboard_id: "d1" })} />,
      route
    )
    expect(await screen.findByText("Pinned to the dashboard")).toBeVisible()
  })

  it("shows a finding pinned before observation pins as pinned", async () => {
    stubObservation({ dashboard_id: "d1", pinned: false, can_pin: true })
    renderWithProviders(
      <PinAction
        finding={finding({ dashboard_id: "d1", annotation_id: "a1" })}
      />,
      route
    )
    expect(await screen.findByText("Pinned to the dashboard")).toBeVisible()
  })
})
