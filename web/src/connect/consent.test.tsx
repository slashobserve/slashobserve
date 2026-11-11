import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { ConnectionRequest } from "./queries"
import { renderWithProviders } from "@/test/render"

import { ConsentPage } from "./consent"

const requestId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"

function pending(
  overrides: Partial<ConnectionRequest> = {}
): ConnectionRequest {
  return {
    id: requestId,
    client: {
      id: "client-1",
      name: "Claude Code",
      registered_at: "2026-11-11T10:00:00Z",
      redirect_host: "localhost:51234",
      local: true,
    },
    person: {
      id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
      name: "Ada Lovelace",
      email: "ada@example.com",
      role: "editor",
    },
    replaces_grant: false,
    expires_at: "2026-11-11T10:10:00Z",
    ...overrides,
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

type Reply = { get?: Response; approve?: Response; deny?: Response }

function mockServer(reply: Reply) {
  const calls: { method: string; path: string }[] = []
  const fetch = vi.fn((request: Request) => {
    const path = new URL(request.url).pathname
    calls.push({ method: request.method, path })
    const base = `/api/v1/oauth/requests/${requestId}`
    if (path === base && reply.get) return Promise.resolve(reply.get)
    if (path === `${base}/approve` && reply.approve) {
      return Promise.resolve(reply.approve)
    }
    if (path === `${base}/deny` && reply.deny) {
      return Promise.resolve(reply.deny)
    }
    return Promise.resolve(new Response(null, { status: 500 }))
  })
  vi.stubGlobal("fetch", fetch)
  return calls
}

function renderPage() {
  return renderWithProviders(<ConsentPage requestId={requestId} />, {
    route: { path: "/connect/$requestId", url: `/connect/${requestId}` },
  })
}

const assign = vi.fn()

beforeEach(() => {
  vi.spyOn(window.location, "assign").mockImplementation(assign)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  assign.mockClear()
})

describe("ConsentPage", () => {
  it("shows the registered name, the person, the role, the return host and the caveats", async () => {
    mockServer({ get: json(pending()) })
    renderPage()
    expect(
      await screen.findByRole("heading", {
        name: "Connect Claude Code to SlashObserve",
      })
    ).toBeTruthy()
    expect(
      screen.getByText(
        'Claude Code will act as you, Ada Lovelace (ada@example.com), with your role (Editor) and your folder access. It can read and change everything you can, and what it changes shows as "Ada Lovelace (via Claude Code)".'
      )
    ).toBeTruthy()
    expect(
      screen.getByText(
        "After you approve, your browser returns to localhost:51234."
      )
    ).toBeTruthy()
    expect(
      screen.getByText(
        "SlashObserve cannot verify the name a client gives. You can disconnect it at any time in Settings."
      )
    ).toBeTruthy()
  })

  it("names an unnamed client and labels it as MCP", async () => {
    const base = pending()
    mockServer({
      get: json(pending({ client: { ...base.client, name: null } })),
    })
    renderPage()
    expect(
      await screen.findByRole("heading", {
        name: "Connect an unnamed MCP client to SlashObserve",
      })
    ).toBeTruthy()
    expect(screen.getByText(/shows as "Ada Lovelace \(via MCP\)"/)).toBeTruthy()
  })

  it("warns about a local client and only then", async () => {
    const base = pending()
    mockServer({
      get: json(pending({ client: { ...base.client, local: false } })),
    })
    renderPage()
    await screen.findByRole("button", { name: "Approve" })
    expect(screen.queryByText(/on this computer/)).toBeNull()
  })

  it("shows the local warning", async () => {
    mockServer({ get: json(pending()) })
    renderPage()
    expect(
      await screen.findByText(
        "Claude Code receives this approval on this computer. Approve only if you just started connecting it yourself: any program on this computer can ask under any name."
      )
    ).toBeTruthy()
  })

  it("says approving replaces an earlier connection", async () => {
    mockServer({ get: json(pending({ replaces_grant: true })) })
    renderPage()
    expect(
      await screen.findByText(
        "You connected this client before. Approving replaces that connection."
      )
    ).toBeTruthy()
  })

  it("approves through the consent API, then follows the returned address", async () => {
    const returned = "http://localhost:51234/callback?code=soc_x&state=s1"
    const calls = mockServer({
      get: json(pending()),
      approve: json({ redirect_to: returned }),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Approve" }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith(returned))
    expect(calls).toContainEqual({
      method: "POST",
      path: `/api/v1/oauth/requests/${requestId}/approve`,
    })
    expect(
      (screen.getByRole("button", { name: "Approve" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "Deny" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
  })

  it("denies through the consent API, then follows the returned address", async () => {
    const returned = "http://localhost:51234/callback?error=access_denied"
    const calls = mockServer({
      get: json(pending()),
      deny: json({ redirect_to: returned }),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Deny" }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith(returned))
    expect(calls.some((call) => call.path.endsWith("/approve"))).toBe(false)
  })

  it("does not follow an address that is not http or https", async () => {
    mockServer({
      get: json(pending()),
      approve: json({ redirect_to: "javascript:alert(1)" }),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Approve" }))
    expect(await screen.findByText(/will not open/)).toBeTruthy()
    expect(assign).not.toHaveBeenCalled()
  })

  it("shows a dead end without Approve for an expired or unknown request", async () => {
    mockServer({
      get: json({ error: "not found", code: "not_found" }, 404),
    })
    renderPage()
    expect(
      await screen.findByText(
        "This request expired or was already answered. Start connecting again from your agent."
      )
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Deny" })).toBeNull()
  })

  it("turns into the dead end when the answer finds the request gone", async () => {
    mockServer({
      get: json(pending()),
      approve: json({ error: "not found", code: "not_found" }, 404),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Approve" }))
    expect(await screen.findByText(/already answered/)).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull()
    expect(assign).not.toHaveBeenCalled()
  })

  it("keeps the request answerable when the person has too many agents", async () => {
    mockServer({
      get: json(pending()),
      approve: json(
        {
          error:
            "You have 20 connected agents. Disconnect one in Settings, then try again.",
          code: "too_many_grants",
        },
        409
      ),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Approve" }))
    expect(
      await screen.findByText(
        "You have 20 connected agents. Disconnect one in Settings, then try again."
      )
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Open Settings" }).getAttribute("href")
    ).toBe("/settings#connected-agents")
    expect(
      (screen.getByRole("button", { name: "Deny" }) as HTMLButtonElement)
        .disabled
    ).toBe(false)
    expect(assign).not.toHaveBeenCalled()
  })

  it("offers Retry when the request cannot be loaded", async () => {
    mockServer({ get: new Response(null, { status: 500 }) })
    renderPage()
    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull()
  })
})
