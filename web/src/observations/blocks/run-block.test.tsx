import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Block } from "@/observations/queries"
import type { ObserveRequest } from "@/observe/queries"
import { renderWithProviders } from "@/test/render"

import { RunBlock } from "./run-block"

const runId = "7a1d2b3c-4d5e-4f60-8a9b-0c1d2e3f4a5b"

function runBlock(cites: string[], overrides: Partial<Block> = {}): Block {
  return {
    id: "b-run",
    kind: "run",
    run_id: runId,
    body: { run_id: runId, cites },
    ...overrides,
  } as unknown as Block
}

function stubRequest(request: Partial<ObserveRequest>, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify(
            status === 200
              ? { request }
              : { error: "not found", code: "not_found" }
          ),
          { status, headers: { "Content-Type": "application/json" } }
        )
    )
  )
}

const running: Partial<ObserveRequest> = {
  id: runId,
  status: "running",
  state: "running",
  can_cancel: true,
  steps: [],
}

const succeeded: Partial<ObserveRequest> = {
  id: runId,
  status: "succeeded",
  state: "succeeded",
  can_cancel: false,
  steps: [],
}

function renderRun(block: Block, onCite = vi.fn(), blocks: Block[] = []) {
  const view = renderWithProviders(
    <RunBlock block={block} blocks={blocks} onCite={onCite} />,
    {
      route: { path: "/", url: "/" },
    }
  )
  return { ...view, onCite }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("RunBlock", () => {
  it("shows the run working with a cancel button when the run can be canceled", async () => {
    stubRequest(running)
    renderRun(runBlock([]))
    expect(await screen.findByText("Observe Agent is working")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Cancel run" })).toBeTruthy()
  })

  it("offers no cancel button when the run cannot be canceled", async () => {
    stubRequest({ ...running, can_cancel: false })
    renderRun(runBlock([]))
    await screen.findByText("Observe Agent is working")
    expect(screen.queryByRole("button", { name: "Cancel run" })).toBeNull()
  })

  it("lists each cited block as a button that scrolls to it", async () => {
    stubRequest(succeeded)
    const { onCite, user } = renderRun(runBlock(["text-1", "panel-1"]))
    expect(await screen.findByText("Observe Agent finished")).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "Run details" }))

    await user.click(screen.getByRole("button", { name: "Conclusion" }))
    await user.click(screen.getByRole("button", { name: "Panel 1" }))

    expect(onCite.mock.calls).toEqual([["text-1"], ["panel-1"]])
  })

  it("names each cited block by the finding's heading and the panel's title", async () => {
    stubRequest(succeeded)
    const cited = [
      {
        id: "text-1",
        kind: "text",
        body: { markdown: "## Checkout latency doubled\n\nBecause." },
      },
      {
        id: "panel-1",
        kind: "panel",
        body: { panel: { title: "p99 latency" } },
      },
      { id: "panel-2", kind: "panel", body: { panel: { type: "stat" } } },
    ] as unknown as Block[]
    const { user } = renderRun(
      runBlock(["text-1", "panel-1", "panel-2"]),
      vi.fn(),
      cited
    )
    expect(await screen.findByText("Observe Agent finished")).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "Run details" }))

    expect(
      screen.getByRole("button", { name: "Checkout latency doubled" })
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "p99 latency" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "Panel 2" })).toBeTruthy()
  })

  it("folds a finished run to its outcome and a link to the run, and unfolds it", async () => {
    stubRequest(succeeded)
    const { user } = renderRun(runBlock(["text-1"]))
    expect(await screen.findByText("Observe Agent finished")).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "View the run" }).getAttribute("href")
    ).toBe(`/c/${runId}`)
    expect(screen.queryByText("Show steps")).toBeNull()
    expect(screen.queryByText("Cites")).toBeNull()

    const control = screen.getByRole("button", { name: "Run details" })
    expect(control.getAttribute("aria-expanded")).toBe("false")
    await user.click(control)
    expect(control.getAttribute("aria-expanded")).toBe("true")
    expect(screen.getByText("Show steps")).toBeTruthy()
    expect(screen.getByText("Cites")).toBeTruthy()

    await user.click(control)
    expect(screen.queryByText("Show steps")).toBeNull()
  })

  it("keeps a run that is still working open, with no fold control", async () => {
    stubRequest(running)
    renderRun(runBlock([]))
    await screen.findByText("Observe Agent is working")
    expect(screen.queryByRole("button", { name: "Run details" })).toBeNull()
    expect(screen.queryByRole("link", { name: "View the run" })).toBeNull()
  })

  it("shows no cites while the run has not saved its finding", async () => {
    stubRequest(succeeded)
    renderRun(runBlock([]))
    await screen.findByText("Observe Agent finished")
    expect(screen.queryByText("Cites")).toBeNull()
  })

  it("says the run stopped when it was canceled", async () => {
    stubRequest({
      ...succeeded,
      status: "failed",
      state: "canceled",
      failure_code: "canceled",
    })
    renderRun(runBlock([]))
    expect(await screen.findByText("Observe Agent stopped")).toBeTruthy()
  })

  it("says the run could not finish and explains why when it failed", async () => {
    stubRequest({
      ...succeeded,
      status: "failed",
      state: "failed",
      failure_code: "rate_limited",
      error: "provider said slow down",
      kind: "investigate",
      intent: "ask",
      dashboard_id: null,
      continued_by: null,
      retried_by: null,
      allowed_actions: [],
    })
    const { user } = renderRun(runBlock([]))
    expect(
      await screen.findByText("Observe Agent could not finish")
    ).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "Run details" }))
    expect(screen.getByRole("alert").textContent).toContain(
      "The model provider is limiting requests"
    )
  })

  it("says the run is gone when the run no longer exists", async () => {
    stubRequest({}, 404)
    renderRun(runBlock([]))
    expect(
      await screen.findByText("This run is no longer available.")
    ).toBeTruthy()
  })

  it("says the run is gone when the block lost its run", async () => {
    renderRun(runBlock([], { run_id: null }))
    expect(
      await screen.findByText("This run is no longer available.")
    ).toBeTruthy()
  })

  it("says the run is gone when the body cannot be read", async () => {
    renderRun(runBlock([], { body: { cites: [] } }))
    expect(
      await screen.findByText("This run is no longer available.")
    ).toBeTruthy()
  })
})
