import { screen, waitFor } from "@testing-library/react"
import { createRef } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { serviceAccountKeys } from "@/api/keys"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { CreateKeyPanel } from "./create-key-panel"
import type { ServiceAccount } from "./queries"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "admin-1", role: "admin" } }),
}))

function account(over: Partial<ServiceAccount> = {}): ServiceAccount {
  return {
    id: "a1",
    name: "ci-bot",
    description: "",
    created_by: "admin-1",
    created_at: "2026-11-01T00:00:00Z",
    role: "editor",
    provisioned: false,
    takes_runs: false,
    presence: "offline",
    last_seen_at: null,
    ...over,
  }
}

type Call = { method: string; url: string; body: unknown }

function serve(current: ServiceAccount) {
  const calls: Call[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const body =
        request.method === "GET" ? null : ((await request.json()) as unknown)
      calls.push({ method: request.method, url: request.url, body })
      const json = (value: unknown, status = 200) =>
        new Response(JSON.stringify(value), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      if (request.method === "PATCH") {
        return json({ ...current, ...(body as object) })
      }
      if (request.method === "POST" && request.url.endsWith("/keys")) {
        return json(
          { token: "sok_secret", key: { id: "k1", name: "laptop" } },
          201
        )
      }
      return json({})
    })
  )
  return calls
}

function renderPanel(current: ServiceAccount) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(serviceAccountKeys.list(), { items: [current] })
  const host = document.createElement("div")
  document.body.append(host)
  return renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <CreateKeyPanel
        onClose={() => undefined}
        nameInput={createRef()}
        copyButton={createRef()}
      />
    </PanelHostContext.Provider>,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  document.body.replaceChildren()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("CreateKeyPanel takes agent runs", () => {
  it("shows the switch off for an account that does not take runs", async () => {
    serve(account())
    renderPanel(account())
    const toggle = await screen.findByRole("switch", {
      name: "Takes agent runs",
    })
    expect(toggle).not.toBeChecked()
    expect(toggle).not.toHaveAttribute("aria-disabled", "true")
  })

  it("sends takes_runs with the new key and nothing else when only the switch changed", async () => {
    const calls = serve(account())
    const { user } = renderPanel(account())
    await user.click(
      await screen.findByRole("switch", { name: "Takes agent runs" })
    )
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
    )
    const patches = calls.filter((call) => call.method === "PATCH")
    expect(patches).toHaveLength(1)
    expect(patches[0].url).toMatch(/\/service-accounts\/a1$/)
    expect(patches[0].body).toEqual({ takes_runs: true })
  })

  it("turns an account that takes runs off", async () => {
    const current = account({ takes_runs: true })
    const calls = serve(current)
    const { user } = renderPanel(current)
    const toggle = await screen.findByRole("switch", {
      name: "Takes agent runs",
    })
    expect(toggle).toBeChecked()
    await user.click(toggle)
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
    )
    expect(calls.find((call) => call.method === "PATCH")?.body).toEqual({
      takes_runs: false,
    })
  })

  it("sends no update when neither role nor switch changed", async () => {
    const calls = serve(account())
    const { user } = renderPanel(account())
    await screen.findByRole("switch", { name: "Takes agent runs" })
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
    )
    expect(calls.some((call) => call.method === "PATCH")).toBe(false)
  })

  it("locks the switch for an account from the service-accounts file", async () => {
    const current = account({ provisioned: true, takes_runs: true })
    serve(current)
    renderPanel(current)
    expect(
      await screen.findByRole("switch", { name: "Takes agent runs" })
    ).toHaveAttribute("aria-disabled", "true")
    expect(
      screen.getByText(/Set in the service-accounts file/)
    ).toBeInTheDocument()
  })
})
