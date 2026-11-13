import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import {
  ConnectedAgentsSection,
  EveryonesAgentsCard,
} from "./connected-agents-card"
import type { ConnectedAgent } from "./rows"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

const route = { path: "/settings", url: "/settings" }

const grantId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"

function grant(mode: ConnectedAgent["mode"]): ConnectedAgent {
  return {
    id: grantId,
    client: {
      id: "c1",
      name: "Claude Code",
      redirect_host: "localhost:3118",
      local: true,
    },
    person: { id: "u1", name: "Ada Lovelace", email: "ada@example.com" },
    created_at: "2026-11-11T10:00:00Z",
    last_used_at: null,
    expires_at: "2027-02-09T10:00:00Z",
    state: "active",
    mode,
    presence: "offline",
    last_seen_at: null,
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer(options: { patch?: () => Response } = {}) {
  let mode: ConnectedAgent["mode"] = "direct"
  const patches: { path: string; body: unknown }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "PATCH") {
        patches.push({ path: pathname, body: await request.clone().json() })
        if (options.patch !== undefined) return options.patch()
        mode = "propose"
        return json(grant(mode))
      }
      if (pathname === "/api/v1/me/grants" || pathname === "/api/v1/grants") {
        return json({ grants: [grant(mode)], next_cursor: null })
      }
      return json({ error: "unexpected", code: "unexpected" }, 500)
    })
  )
  return patches
}

beforeEach(() => {
  toasts.error.mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("ModeSelect", () => {
  it("sends propose for the person's own connection and shows the server's answer", async () => {
    const patches = mockServer()
    const { user } = renderWithProviders(<ConnectedAgentsSection />, { route })
    const trigger = await screen.findByRole("combobox", {
      name: "Mode for Claude Code",
    })
    expect(trigger.textContent).toContain("Direct")
    await user.click(trigger)
    await user.click(await screen.findByRole("option", { name: /Propose/ }))
    await waitFor(() =>
      expect(patches).toEqual([
        { path: `/api/v1/me/grants/${grantId}`, body: { mode: "propose" } },
      ])
    )
    await waitFor(() =>
      expect(
        screen.getByRole("combobox", { name: "Mode for Claude Code" })
          .textContent
      ).toContain("Propose")
    )
  })

  it("describes each mode in the list", async () => {
    mockServer()
    const { user } = renderWithProviders(<ConnectedAgentsSection />, { route })
    await user.click(
      await screen.findByRole("combobox", { name: "Mode for Claude Code" })
    )
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "DirectSaves change dashboards",
      "ProposeSaves become proposals an editor merges",
    ])
  })

  it("keeps the current mode and says so when the change fails", async () => {
    mockServer({
      patch: () =>
        json({ error: "connection not found", code: "not_found" }, 404),
    })
    const { user } = renderWithProviders(<ConnectedAgentsSection />, { route })
    await user.click(
      await screen.findByRole("combobox", { name: "Mode for Claude Code" })
    )
    await user.click(await screen.findByRole("option", { name: /Propose/ }))
    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Connection not found.")
    )
    expect(
      screen.getByRole("combobox", { name: "Mode for Claude Code" }).textContent
    ).toContain("Direct")
  })

  it("shows the mode as text, not a select, on the everyone's list", async () => {
    mockServer()
    renderWithProviders(<EveryonesAgentsCard />)
    const everyone = await screen.findByRole("region", {
      name: "Everyone's connected agents",
    })
    await waitFor(() =>
      expect(within(everyone).getByText("Direct")).toBeTruthy()
    )
    expect(screen.queryByRole("combobox")).toBeNull()
  })

  it("keeps the everyone's list out of the person's own section", async () => {
    mockServer()
    renderWithProviders(<ConnectedAgentsSection />, { route })
    await screen.findByRole("combobox", { name: "Mode for Claude Code" })
    expect(
      screen.queryByRole("region", { name: "Everyone's connected agents" })
    ).toBeNull()
    expect(screen.getAllByRole("combobox")).toHaveLength(1)
  })
})
