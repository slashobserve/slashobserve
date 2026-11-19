import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Role } from "@/lib/roles"
import { testQueryClient } from "@/test/render"

import { FolderAccessCard } from "./folder-access-card"

const folderId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function person(id: string, name: string, email: string) {
  return {
    id,
    name,
    email,
    created_at: "2026-11-01T00:00:00Z",
    last_login_at: "2026-11-01T00:00:00Z",
    last_seen_at: "2026-11-01T00:00:00Z",
    dashboard_count: 0,
    key_count: 0,
    session_count: 0,
    role: "editor",
    role_source: "default",
    role_locked: false,
  }
}

function renderCard(role: Role, entries: unknown[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname
      if (path === `/api/v1/folders/${folderId}/permissions`) {
        return json({ entries })
      }
      if (path === "/api/v1/users") {
        return json({
          users: [
            person("1", "Edie", "editor@example.com"),
            person("2", "Vic", "viewer@example.com"),
          ],
          next_cursor: null,
        })
      }
      if (path === "/api/v1/instance")
        return json({ oidc_provider_name: "Dex" })
      return new Response("{}", { status: 404 })
    })
  )
  const root = createRootRoute()
  const authed = createRoute({
    getParentRoute: () => root,
    id: "_authed",
    beforeLoad: () => ({
      principal: { kind: "user", id: "9", name: "Ana", role },
    }),
  })
  const page = createRoute({
    getParentRoute: () => authed,
    path: "/",
    component: () => <FolderAccessCard folderId={folderId} readOnly={false} />,
  })
  const router = createRouter({
    routeTree: root.addChildren([authed.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={testQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return user
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("FolderAccessCard", () => {
  it("says an empty restriction restricts nothing until someone is added", async () => {
    const user = renderCard("admin")
    await user.click(
      await screen.findByRole("radio", {
        name: /Only the people and groups below/,
      })
    )
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Nobody is listed yet, so everyone still sees this folder by role."
    )
  })

  it("suggests the people who have signed in to an Admin typing an address", async () => {
    const user = renderCard("admin", [
      {
        kind: "user",
        level: "view",
        user: {
          id: "2",
          name: "Vic",
          email: "viewer@example.com",
          role: "viewer",
        },
      },
    ])
    const field = await screen.findByLabelText("Add a person or group")
    await user.type(field, "e")
    expect(
      await screen.findByRole("option", { name: "editor@example.com" })
    ).toBeVisible()
    expect(
      screen.queryByRole("option", { name: "viewer@example.com" })
    ).not.toBeInTheDocument()
  })

  it("offers no suggestions to someone who may not list people", async () => {
    const user = renderCard("editor")
    await user.click(
      await screen.findByRole("radio", {
        name: /Only the people and groups below/,
      })
    )
    await user.type(await screen.findByLabelText("Add a person or group"), "e")
    expect(screen.queryByRole("option")).not.toBeInTheDocument()
  })
})
