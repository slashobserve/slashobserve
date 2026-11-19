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

import { outboundDomainKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import { OutboundAccessSection } from "./outbound-access-card"

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function domains(usedBy: { kind: string; name: string }[]) {
  return {
    managed_by: "api",
    domains: [
      {
        domain: "hooks.example.com",
        added_at: "2026-11-18T10:00:00Z",
        added_by: "Ada",
        from_connection: false,
        used_by: usedBy,
      },
    ],
  }
}

afterEach(() => vi.unstubAllGlobals())

describe("OutboundAccessSection", () => {
  it("names the webhook that was added after the list loaded when asking to stop allowing a domain", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json(domains([{ kind: "webhook", name: "admin-hook" }]))
      )
    )
    const queryClient = testQueryClient()
    queryClient.setQueryData(outboundDomainKeys.list(), domains([]))
    const root = createRootRoute()
    const authed = createRoute({
      getParentRoute: () => root,
      id: "_authed",
      beforeLoad: () => ({
        principal: { kind: "user", id: "9", name: "Ana", role: "admin" },
      }),
    })
    const page = createRoute({
      getParentRoute: () => authed,
      path: "/",
      component: () => <OutboundAccessSection />,
    })
    const router = createRouter({
      routeTree: root.addChildren([authed.addChildren([page])]),
      history: createMemoryHistory({ initialEntries: ["/"] }),
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )

    await user.click(
      await screen.findByRole("button", { name: /Remove hooks.example.com/ })
    )

    expect(
      await screen.findByText(/Webhook admin-hook can no longer reach it/)
    ).toBeTruthy()
  })
})
