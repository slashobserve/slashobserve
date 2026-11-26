import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { outboundDomainKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import { OutboundAccessSection } from "./outbound-access-card"

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
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

function renderSection(queryClient: ReturnType<typeof testQueryClient>) {
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
  return user
}

const refusal = "An IP address is not allowed. Use a domain name."

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
    const user = renderSection(queryClient)

    await user.click(
      await screen.findByRole("button", { name: /Remove hooks.example.com/ })
    )

    expect(
      await screen.findByText(/Webhook admin-hook can no longer reach it/)
    ).toBeTruthy()
  })

  describe("a refused domain", () => {
    function refuseAdds() {
      vi.stubGlobal(
        "fetch",
        vi.fn(async (request: Request) => {
          if (request.method === "POST") {
            return json({ error: refusal, code: "invalid_domain" }, 422)
          }
          if (request.method === "DELETE")
            return new Response(null, { status: 204 })
          return json(domains([]))
        })
      )
      const queryClient = testQueryClient()
      queryClient.setQueryData(outboundDomainKeys.list(), domains([]))
      return renderSection(queryClient)
    }

    it("is cleared when the field empties", async () => {
      const user = refuseAdds()
      const field = await screen.findByLabelText("Domain")
      await user.type(field, "10.0.0.1")
      await user.click(screen.getByRole("button", { name: "Allow domain" }))
      expect(await screen.findByText(refusal)).toBeTruthy()

      await user.clear(field)

      expect(screen.queryByText(refusal)).toBeNull()
    })

    it("is cleared by a later successful removal", async () => {
      const user = refuseAdds()
      await user.type(await screen.findByLabelText("Domain"), "10.0.0.1")
      await user.click(screen.getByRole("button", { name: "Allow domain" }))
      expect(await screen.findByText(refusal)).toBeTruthy()

      await user.click(
        screen.getByRole("button", { name: /Remove hooks.example.com/ })
      )
      await user.click(
        await screen.findByRole("button", { name: "Remove domain" })
      )

      await waitFor(() => expect(screen.queryByText(refusal)).toBeNull())
    })
  })
})
