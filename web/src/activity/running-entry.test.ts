import { screen } from "@testing-library/react"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { afterEach, describe, expect, it, vi } from "vitest"

import { observeKeys } from "@/api/keys"
import { redactedRun } from "@/test/agent-request"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { RunningEntry, RunningLinkContent } from "./running-entry"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

afterEach(() => vi.unstubAllGlobals())

describe("RunningLinkContent", () => {
  const markup = renderToStaticMarkup(
    createElement(RunningLinkContent, { stepText: "Running queries, step 3" })
  )

  it("keeps the current step in the link's accessible text", () => {
    expect(markup).toContain("Running queries, step 3")
    expect(markup).not.toContain("aria-label")
  })

  it("adds the purpose and the new tab hint as screen reader text", () => {
    expect(markup).toContain(
      '<span class="sr-only"> of the running agent request, opens in a new tab</span>'
    )
  })
})

describe("RunningEntry", () => {
  it("shows a running run from someone else's private chat by its phase, without step messages", async () => {
    const step = {
      attempt: 0,
      phase: "discover" as const,
      status: "ok" as const,
      message: "",
      duration_ms: 10,
    }
    const run = redactedRun({
      steps: [
        { ...step, at: "2026-11-21T10:00:10Z", tool: "list_metrics" },
        { ...step, at: "2026-11-21T10:00:20Z", tool: "get_schema" },
      ],
    })
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ request: run }))
    )
    const queryClient = testQueryClient()
    queryClient.setQueryData(observeKeys.request(run.id), run)
    renderWithProviders(
      createElement(RunningEntry, {
        entry: {
          kind: "running",
          key: `running:${run.id}`,
          at: run.created_at,
          panelId: null,
          request: run,
        },
      }),
      { queryClient, route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("link", { name: /^Discovering, step 2 Details/ })
    ).toBeVisible()
    expect(screen.getByText("list_metrics, get_schema")).toBeVisible()
    expect(screen.queryByRole("button", { name: /steps$/ })).toBeNull()
  })
})
