import { QueryClientProvider, type QueryClient } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  RouterProvider,
  type ErrorRouteComponent,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { parseSearch, stringifySearch } from "@/lib/search"
import { Route as FindingRoute } from "@/routes/_authed/_shell/findings/$findingId"
import { testQueryClient } from "@/test/render"

const findingId = "3f1c9a52-7d0e-4b8a-9c21-5e6f7a8b9c01"
const observationId = "8a2b4c6d-1e3f-4a5b-8c7d-9e0f1a2b3c4d"

const { beforeLoad, errorComponent } = FindingRoute.options as unknown as {
  beforeLoad: (options: {
    params: { findingId: string }
    context: { queryClient: QueryClient }
  }) => Promise<void>
  errorComponent: ErrorRouteComponent
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function renderAt(url: string, queryClient: QueryClient) {
  const root = createRootRouteWithContext<{ queryClient: QueryClient }>()()
  const finding = createRoute({
    getParentRoute: () => root,
    path: "/findings/$findingId",
    beforeLoad,
    errorComponent,
  })
  const observation = createRoute({
    getParentRoute: () => root,
    path: "/o/$observationId",
    component: () => <p>observation page</p>,
  })
  const observations = createRoute({
    getParentRoute: () => root,
    path: "/observations",
    component: () => <p>observation list</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([finding, observation, observations]),
    history: createMemoryHistory({ initialEntries: [url] }),
    context: { queryClient },
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("the /findings/<id> address", () => {
  it("lands on the finding's observation, replacing the old address in history", async () => {
    const requested: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        requested.push(new URL(request.url).pathname)
        return json({
          finding: { id: findingId, observation_id: observationId },
          folder: null,
          permissions: {},
        })
      })
    )
    const router = renderAt(`/findings/${findingId}`, testQueryClient())

    expect(await screen.findByText("observation page")).toBeInTheDocument()
    expect(router.state.location.pathname).toBe(`/o/${observationId}`)
    expect(router.history.length).toBe(1)
    expect(requested).toEqual([`/api/v1/findings/${findingId}`])
  })

  it("says the finding is missing and links to the observations list", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json({ error: { code: "not_found", message: "not found" } }, 404)
      )
    )
    renderAt(`/findings/${findingId}`, testQueryClient())

    expect(
      await screen.findByText(
        "This finding does not exist or you cannot see it"
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: "Back to Observations" })
    ).toHaveAttribute("href", "/observations")
  })

  it("offers a retry on any other failure and follows the finding once it loads", async () => {
    const answers = [
      () => json({ error: { code: "internal", message: "boom" } }, 500),
      () =>
        json({
          finding: { id: findingId, observation_id: observationId },
          folder: null,
          permissions: {},
        }),
    ]
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => answers.shift()?.())
    )
    const queryClient = testQueryClient()
    renderAt(`/findings/${findingId}`, queryClient)

    expect(
      await screen.findByText("Could not load the finding")
    ).toBeInTheDocument()
    screen.getByRole("button", { name: "Retry" }).click()

    expect(await screen.findByText("observation page")).toBeInTheDocument()
  })
})
