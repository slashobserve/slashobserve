import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import { beforeAll, describe, expect, it } from "vitest"

import { AnnotationControls } from "@/annotations/annotation-controls"
import { annotationTagsQuery } from "@/annotations/queries"
import { parseSearch, stringifySearch } from "@/lib/search"
import { testQueryClient } from "@/test/render"

import { DashboardTopRow } from "./dashboard-top-row"

beforeAll(() => {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    }) as unknown as MediaQueryList
})

async function renderRow(controls: ReactNode[]) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(annotationTagsQuery("d1").queryKey, ["deploy"])
  const root = createRootRoute()
  const authed = createRoute({ getParentRoute: () => root, id: "_authed" })
  const dashboard = createRoute({
    getParentRoute: () => authed,
    path: "d/$dashboardId",
  })
  const page = createRoute({
    getParentRoute: () => dashboard,
    path: "{-$slug}",
    component: () => (
      <DashboardTopRow
        title="Checkout"
        madeBy={null}
        versionControl={null}
        controls={controls}
        actions={[]}
      />
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([
      authed.addChildren([dashboard.addChildren([page])]),
    ]),
    history: createMemoryHistory({ initialEntries: ["/d/d1/checkout"] }),
    parseSearch,
    stringifySearch,
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText("Checkout")
}

describe("DashboardTopRow", () => {
  it("shows the annotation filter only when the page passes it", async () => {
    await renderRow([<AnnotationControls key="annotations" />])
    expect(
      await screen.findByRole("button", { name: /Annotations: All/ })
    ).toBeVisible()
  })

  it("shows no annotation menu on a page that passes no controls", async () => {
    await renderRow([])
    expect(screen.queryByRole("button", { name: /Annotations/ })).toBeNull()
  })
})
