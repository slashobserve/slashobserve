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

function renderRun(block: Block, onCite = vi.fn()) {
  const view = renderWithProviders(<RunBlock block={block} onCite={onCite} />, {
    route: { path: "/", url: "/" },
  })
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

    await user.click(screen.getByRole("button", { name: "Conclusion" }))
    await user.click(screen.getByRole("button", { name: "Panel 1" }))

    expect(onCite.mock.calls).toEqual([["text-1"], ["panel-1"]])
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
      source: "investigate",
      dashboard_id: null,
      continued_by: null,
      allowed_actions: [],
    })
    renderRun(runBlock([]))
    expect(
      await screen.findByText("Observe Agent could not finish")
    ).toBeTruthy()
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
