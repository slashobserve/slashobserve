import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, test, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { HealthCell } from "./health-cell"
import type { HealthItem, HealthProblem } from "./health-labels"

const provider = vi.hoisted(() => ({
  item: null as HealthItem | null,
  pending: false,
  requestRepair: vi.fn(),
}))

vi.mock("./list-health", () => ({
  useListHealth: () => ({ item: provider.item, pending: provider.pending }),
  useListHealthRepair: () => ({
    agentAvailable: true,
    pendingId: null,
    requestRepair: provider.requestRepair,
  }),
}))

const queryFailed: HealthProblem = {
  kind: "query_failed",
  panel_id: "1",
  panel_title: "Orders",
  variable: null,
  code: "bad_data",
  message: "parse error",
}

const noData: HealthProblem = {
  kind: "no_data",
  panel_id: "2",
  panel_title: "Latency",
  variable: null,
  code: "query_empty",
  message: "query returned no data",
}

function item(fields: Partial<HealthItem> = {}): HealthItem {
  return {
    id: "d1",
    title: "Checkout",
    url: "/d/d1/checkout",
    folder: { id: "f1", title: "Shop" },
    can_edit: true,
    checked_version: 4,
    checked_at: "2026-11-06T10:00:00Z",
    last_viewed_at: null,
    skipped: null,
    problems: [queryFailed, noData],
    repair: null,
    ...fields,
  } as HealthItem
}

function renderCell() {
  return renderWithProviders(
    <div data-testid="cell">
      <HealthCell dashboardId="d1" title="Checkout" />
    </div>,
    { route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  provider.item = null
  provider.pending = false
  provider.requestRepair.mockReset()
})

describe("HealthCell", () => {
  test("renders nothing for a dashboard with no known problem", async () => {
    renderCell()
    expect((await screen.findByTestId("cell")).textContent).toBe("")
  })

  test("shows a skeleton while the page's health loads", async () => {
    provider.pending = true
    renderCell()
    const cell = await screen.findByTestId("cell")
    expect(cell.textContent).toBe("")
    expect(cell.querySelector("[data-slot=skeleton]")).not.toBeNull()
  })

  test("shows the first problem, how many others there are and a full label", async () => {
    provider.item = item()
    renderCell()
    const trigger = await screen.findByRole("button", {
      name: "Health of Checkout: Query fails and 1 more problem",
    })
    expect(trigger.textContent).toContain("Query fails")
    expect(trigger.textContent).toContain("Query fails·Orders")
    expect(trigger.textContent).toContain("+1 more")
  })

  test("adds the repair status to the trigger and its label", async () => {
    provider.item = item({
      repair: {
        request_id: "0a1b2c3d-0000-4000-8000-0000000000a1",
        state: "running",
        outcome: "running",
        version: null,
        error: null,
        updated_at: "2026-11-06T10:00:00Z",
      },
    } as Partial<HealthItem>)
    renderCell()
    const trigger = await screen.findByRole("button", {
      name: "Health of Checkout: Query fails and 1 more problem, Repairing",
    })
    expect(trigger.textContent).toContain("Repairing")
  })

  test("opens with Enter, moves focus into the popover and returns it on Escape", async () => {
    provider.item = item()
    const { user } = renderCell()
    const trigger = await screen.findByRole("button", { name: /^Health of/ })
    trigger.focus()

    await user.keyboard("{Enter}")

    const popover = await screen.findByRole("dialog")
    expect(popover.textContent).toContain("Health")
    expect(popover.textContent).toContain("Checked")
    expect(popover.textContent).toContain("on version 4")
    expect(popover.contains(document.activeElement)).toBe(true)
    expect(
      screen.getByRole("button", { name: "Repair" }).closest("[role=dialog]")
    ).toBe(popover)

    await user.keyboard("{Escape}")

    expect(screen.queryByRole("dialog")).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  test("says when the dashboard was not checked or its check was skipped", async () => {
    provider.item = item({ checked_at: null, checked_version: null })
    const first = renderCell()
    await first.user.click(
      await screen.findByRole("button", { name: /^Health of/ })
    )
    expect((await screen.findByRole("dialog")).textContent).toContain(
      "Not checked yet"
    )
    first.unmount()

    provider.item = item({ skipped: "depends_on_viewer" })
    const second = renderCell()
    await second.user.click(
      await screen.findByRole("button", { name: /^Health of/ })
    )
    expect((await screen.findByRole("dialog")).textContent).toContain(
      "Skipped: reads the viewer"
    )
  })

  test("asks the provider to repair from the popover", async () => {
    provider.item = item()
    const { user } = renderCell()
    await user.click(await screen.findByRole("button", { name: /^Health of/ }))
    await user.click(await screen.findByRole("button", { name: "Repair" }))
    expect(provider.requestRepair).toHaveBeenCalledWith("d1")
  })
})
