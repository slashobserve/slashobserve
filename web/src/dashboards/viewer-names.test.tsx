import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"

import { ViewerNames, viewersLabel } from "./viewer-names"

const dashboardId = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"

function stubViewers(names: string[]) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(async () =>
    Response.json({
      viewers: names.map((name, index) => ({ id: `id-${index}`, name })),
    })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderNames() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ViewerNames dashboardId={dashboardId} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("viewersLabel", () => {
  test("lists up to three names and counts the rest", () => {
    expect(viewersLabel(["Ada", "Grace"])).toBe("Ada, Grace")
    expect(viewersLabel(["Ada", "Grace", "Linus", "Ken", "Dennis"])).toBe(
      "Ada, Grace, Linus and 2 more"
    )
  })
})

describe("ViewerNames", () => {
  test("reports the open dashboard and shows who else is viewing", async () => {
    const fetch = stubViewers(["Ada", "Grace"])
    renderNames()
    expect(await screen.findByText("Also viewing: Ada, Grace")).toBeTruthy()
    const [request] = fetch.mock.calls[0]!
    expect([request.method, new URL(request.url).pathname]).toEqual([
      "POST",
      `/api/v1/dashboards/${dashboardId}/viewers`,
    ])
  })

  test("renders nothing when nobody else is viewing", async () => {
    const fetch = stubViewers([])
    const { container } = renderNames()
    await vi.waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(container.textContent).toBe("")
  })
})
