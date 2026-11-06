import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Variable } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { PublicLinksSection } from "./public-links-section"

const dashboardId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"
const token = `pub_${"b".repeat(43)}`
const url = `https://observe.example.com/p/${token}`

const link = {
  id: "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77",
  prefix: token.slice(0, 12),
  variables: "frozen",
  selections: { env: ["dev"] },
  expires_at: null,
  created_by: {
    kind: "user",
    id: "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f",
    name: "Ana",
  },
  created_at: "2026-11-06T09:00:00Z",
  last_viewed_at: null,
  expired: false,
  usable: true,
}

const variables: Variable[] = [
  { name: "env", type: "custom", options: ["prod", "dev"] },
]

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer() {
  const posted: unknown[] = []
  let links: unknown[] = []
  const fetch = vi.fn(async (request: Request) => {
    if (request.method === "POST") {
      posted.push(await request.json())
      links = [link]
      return json({ link, token, url }, 201)
    }
    return json({ public_links: links })
  })
  vi.stubGlobal("fetch", fetch)
  return posted
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("PublicLinksSection", () => {
  it("shows a new link's address once and keeps the token out of the caches", async () => {
    const posted = mockServer()
    const { user, queryClient } = renderWithProviders(
      <PublicLinksSection
        dashboardId={dashboardId}
        variables={variables}
        vars={{ env: ["dev"], stray: ["x"] }}
      />
    )
    await user.click(
      await screen.findByRole("button", { name: "Create public link" })
    )

    expect(await screen.findByLabelText("Public link")).toHaveValue(url)
    expect(
      screen.getByText("Copy it now. It is not shown again.")
    ).toBeInTheDocument()
    expect(posted).toEqual([
      { variables: "frozen", selections: { env: ["dev"] }, expires_at: null },
    ])
    expect(
      await screen.findByRole("button", {
        name: `Revoke public link ${link.prefix}`,
      })
    ).toBeInTheDocument()

    await waitFor(() =>
      expect(queryClient.getMutationCache().getAll()).toEqual([])
    )
    const cached = JSON.stringify(
      queryClient
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data)
    )
    expect(cached).toContain(link.prefix)
    expect(cached).not.toContain(token)
  })

  it("sends no selections for a free link", async () => {
    const posted = mockServer()
    const { user } = renderWithProviders(
      <PublicLinksSection
        dashboardId={dashboardId}
        variables={variables}
        vars={{ env: ["dev"] }}
      />
    )
    await user.click(await screen.findByRole("combobox", { name: "Variables" }))
    await user.click(
      await screen.findByRole("option", { name: "Free: viewers choose" })
    )
    await user.click(screen.getByRole("button", { name: "Create public link" }))
    await screen.findByLabelText("Public link")
    expect(posted).toEqual([
      { variables: "free", selections: {}, expires_at: null },
    ])
  })

  it("refuses a missing date before sending anything", async () => {
    const posted = mockServer()
    const { user } = renderWithProviders(
      <PublicLinksSection
        dashboardId={dashboardId}
        variables={variables}
        vars={{}}
      />
    )
    await user.click(await screen.findByRole("combobox", { name: "Expires" }))
    await user.click(await screen.findByRole("option", { name: "On a date" }))
    await user.click(screen.getByRole("button", { name: "Create public link" }))
    expect(await screen.findByText("Choose a date.")).toBeInTheDocument()
    expect(posted).toEqual([])
  })
})
