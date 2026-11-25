import { screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { VersionList } from "./version-list"

vi.mock("./restore-version-button", () => ({
  RestoreVersionButton: ({ version }: { version: number }) => (
    <button type="button">Restore version {version}</button>
  ),
}))

const dashboardId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"

function version(
  number: number,
  extra: Record<string, unknown> = {}
): Record<string, unknown> {
  return {
    version: number,
    summary: `Change ${number}`,
    created_at: "2026-11-06T09:00:00Z",
    restored_from: null,
    agent_request_id: null,
    actor: { kind: "user", label: "Ana" },
    ...extra,
  }
}

function serve(versions: Record<string, unknown>[], status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify(
            status === 200
              ? { versions, next_cursor: null }
              : { error: { code: "internal", message: "boom" } }
          ),
          { status, headers: { "Content-Type": "application/json" } }
        )
    )
  )
}

function renderList(onCompare = vi.fn()) {
  return {
    onCompare,
    ...renderWithProviders(
      <VersionList
        dashboardId={dashboardId}
        latestVersion={3}
        back={undefined}
        onCompare={onCompare}
        onRestored={() => undefined}
      />,
      { route: { path: "/", url: "/" } }
    ),
  }
}

afterEach(() => vi.unstubAllGlobals())

describe("VersionList", () => {
  it("lists each version with its badges, and offers Restore on the older ones only", async () => {
    serve([
      version(3),
      version(2, { restored_from: 1 }),
      version(1, { actor: { kind: "user", label: "Bo" } }),
    ])
    renderList()

    const grid = await screen.findByRole("grid", { name: "Versions" })
    const latest = await within(grid).findByRole("gridcell", { name: /^v3/ })
    expect(within(latest).getByText("Latest")).toBeInTheDocument()
    const restored = within(grid).getByRole("gridcell", { name: /^v2/ })
    expect(within(restored).getByText("Restored from v1")).toBeInTheDocument()
    expect(within(grid).getByText("Change 2")).toBeInTheDocument()
    expect(within(grid).getByText("Bo")).toBeInTheDocument()
    expect(
      within(grid).getByRole("link", { name: "View version 2" })
    ).toBeInTheDocument()
    expect(
      within(grid).getByRole("button", { name: "Restore version 2" })
    ).toBeInTheDocument()
    expect(
      within(grid).queryByRole("button", { name: "Restore version 3" })
    ).toBeNull()
  })

  it("asks to compare the version of the row", async () => {
    serve([version(3), version(2), version(1)])
    const { user, onCompare } = renderList()

    await user.click(
      await screen.findByRole("button", {
        name: "Compare version 1 with an empty dashboard",
      })
    )

    expect(onCompare).toHaveBeenCalledWith(1)
  })

  it("links an agent's version to its chat", async () => {
    serve([version(3, { agent_request_id: "req-1" })])
    renderList()

    expect(
      await screen.findByRole("link", { name: "From chat for version 3" })
    ).toHaveAttribute("href", "/c/req-1")
  })

  it("shows a retryable problem instead of the grid when the load fails", async () => {
    serve([], 500)
    renderList()

    expect(await screen.findByText("Could not load versions")).toBeVisible()
    expect(screen.queryByRole("grid")).toBeNull()
  })
})
