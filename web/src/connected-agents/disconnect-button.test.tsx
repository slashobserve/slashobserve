import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { DisconnectButton } from "./disconnect-button"
import type { ConnectedAgent } from "./rows"

const grant: ConnectedAgent = {
  id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
  client: {
    id: "c1",
    name: "Claude Code",
    redirect_host: "localhost:3118",
    local: true,
  },
  person: {
    id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
    name: "Ada Lovelace",
    email: "ada@example.com",
  },
  created_at: "2026-11-11T10:00:00Z",
  last_used_at: null,
  expires_at: "2027-02-09T10:00:00Z",
  state: "active",
  mode: "direct",
  presence: "offline",
  last_seen_at: null,
}

function mockServer(status: number) {
  const calls: { method: string; path: string }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      calls.push({
        method: request.method,
        path: new URL(request.url).pathname,
      })
      return Promise.resolve(new Response(null, { status }))
    })
  )
  return calls
}

function renderButton(scope: "mine" | "all") {
  return renderWithProviders(<DisconnectButton grant={grant} scope={scope} />)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("DisconnectButton", () => {
  it("asks first and sends nothing until confirmed", async () => {
    const calls = mockServer(204)
    const { user } = renderButton("mine")
    await user.click(
      screen.getByRole("button", { name: "Disconnect Claude Code" })
    )
    expect(screen.getByText("Disconnect Claude Code?")).toBeTruthy()
    expect(
      screen.getByText(/It stops working at once\. Connect it again/)
    ).toBeTruthy()
    expect(calls).toEqual([])
  })

  it("disconnects your own agent through the person route", async () => {
    const calls = mockServer(204)
    const { user } = renderButton("mine")
    await user.click(
      screen.getByRole("button", { name: "Disconnect Claude Code" })
    )
    const dialog = screen.getByRole("alertdialog")
    await user.click(within(dialog).getByRole("button", { name: "Disconnect" }))
    await waitFor(() =>
      expect(calls).toEqual([
        { method: "DELETE", path: `/api/v1/me/grants/${grant.id}` },
      ])
    )
  })

  it("disconnects anyone's agent through the admin route", async () => {
    const calls = mockServer(204)
    const { user } = renderButton("all")
    await user.click(
      screen.getByRole("button", {
        name: "Disconnect Claude Code of Ada Lovelace",
      })
    )
    await user.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "Disconnect",
      })
    )
    await waitFor(() =>
      expect(calls).toEqual([
        { method: "DELETE", path: `/api/v1/grants/${grant.id}` },
      ])
    )
  })

  it("shows the problem and keeps the dialog open when the server fails", async () => {
    mockServer(500)
    const { user } = renderButton("mine")
    await user.click(
      screen.getByRole("button", { name: "Disconnect Claude Code" })
    )
    await user.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "Disconnect",
      })
    )
    expect(
      await within(screen.getByRole("alertdialog")).findByRole("alert")
    ).toBeTruthy()
  })
})
