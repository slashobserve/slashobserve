import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AskOnDashboard } from "./ask-on-dashboard"

const permissions = vi.hoisted(() => ({ edit: true }))

vi.mock("@/dashboards/permissions", () => ({
  useLatestPermissions: () => ({
    comment: true,
    edit: permissions.edit,
    manage: false,
    create: false,
  }),
}))

const askOn = {
  dashboard_id: "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11",
  title: "Checkout",
}
const prompt = "On the dashboard Checkout add a stat panel of the error rate"

function stubCreate() {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async () =>
      new Response(JSON.stringify({ request: { id: "r2", prompt } }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderAsk() {
  return renderWithProviders(<AskOnDashboard askOn={askOn} prompt={prompt} />, {
    route: { path: "/", url: "/" },
  })
}

describe("AskOnDashboard", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    permissions.edit = true
  })

  it("asks the same request on the named dashboard and opens the new run", async () => {
    const fetch = stubCreate()
    const { user, router } = renderAsk()
    expect(
      await screen.findByText(/This change is made on the dashboard Checkout/)
    ).toBeVisible()
    await user.click(
      screen.getByRole("button", { name: "Ask this on Checkout" })
    )
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    const sent = fetch.mock.calls[0][0]
    expect(new URL(sent.url).pathname).toBe("/api/v1/observe")
    expect(await sent.json()).toEqual({
      prompt,
      dashboard_id: askOn.dashboard_id,
      time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    })
    await waitFor(() => expect(router?.state.location.pathname).toBe("/c/r2"))
  })

  it("links to the dashboard and offers no ask to someone who cannot edit it", async () => {
    permissions.edit = false
    const fetch = stubCreate()
    renderAsk()
    expect(await screen.findByText(/which you cannot edit/)).toBeVisible()
    expect(
      screen.queryByRole("button", { name: /Ask this on/ })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: "Open dashboard" })
    ).toHaveAttribute("href", `/d/${askOn.dashboard_id}/checkout`)
    expect(fetch).not.toHaveBeenCalled()
  })
})
