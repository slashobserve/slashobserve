import { screen, waitFor } from "@testing-library/react"
import { describe, expect, test, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { HealthItem, HealthProblem } from "./health-labels"
import { PanelHealthBadge } from "./panel-health-badge"

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
  panel_id: "1",
  panel_title: "Orders",
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
    state: "failing",
    checked_version: 4,
    checked_at: "2026-11-06T10:00:00Z",
    last_viewed_at: null,
    skipped: null,
    problems: [queryFailed],
    repair: null,
    ...fields,
  } as HealthItem
}

function renderBadge(
  props: Partial<Parameters<typeof PanelHealthBadge>[0]> = {}
) {
  const onRepair = vi.fn()
  const result = renderWithProviders(
    <PanelHealthBadge
      title="Orders"
      problems={[queryFailed]}
      item={item()}
      agentAvailable
      pending={false}
      onRepair={onRepair}
      blockedReason={null}
      {...props}
    />,
    { route: { path: "/", url: "/" } }
  )
  return { ...result, onRepair }
}

describe("PanelHealthBadge", () => {
  test("carries the panel, the kind and the message in its label", async () => {
    renderBadge()
    const badge = await screen.findByRole("button", {
      name: "Orders: Query fails. Parse error.",
    })
    expect(badge.hasAttribute("data-health")).toBe(true)
    expect(badge.textContent).toBe("Query fails")
  })

  test("leads with the worst problem and lists every one in the popover", async () => {
    const { user } = renderBadge({ problems: [noData, queryFailed] })
    const badge = await screen.findByRole("button", {
      name: "Orders: Query fails. Parse error.",
    })
    await user.click(badge)
    expect(await screen.findByText("Parse error.")).toBeTruthy()
    expect(screen.getByText("No data")).toBeTruthy()
    expect(screen.getByText("No data in the last 7 days.")).toBeTruthy()
  })

  test("opens with Enter, says when it checked, and returns focus on Escape", async () => {
    const { user } = renderBadge()
    const badge = await screen.findByRole("button", { name: /^Orders:/ })
    badge.focus()
    await user.keyboard("{Enter}")
    expect(await screen.findByText(/^Checked/)).toBeTruthy()
    expect(
      screen.getByText(
        "Repair asks the agent to fix every failing panel and variable on this dashboard in one revision."
      )
    ).toBeTruthy()
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByText(/^Checked/)).toBeNull())
    expect(document.activeElement).toBe(badge)
  })

  test("repairs through onRepair", async () => {
    const { user, onRepair } = renderBadge()
    await user.click(await screen.findByRole("button", { name: /^Orders:/ }))
    await user.click(await screen.findByRole("button", { name: "Repair" }))
    expect(onRepair).toHaveBeenCalledTimes(1)
  })

  test("disables Repair with the blocked reason", async () => {
    const { user, onRepair } = renderBadge({
      blockedReason: "finish the layout first",
    })
    await user.click(await screen.findByRole("button", { name: /^Orders:/ }))
    const repair = await screen.findByRole("button", { name: "Repair" })
    expect(repair.getAttribute("aria-disabled")).toBe("true")
    expect(repair.getAttribute("title")).toBe("finish the layout first")
    await user.click(repair)
    expect(onRepair).not.toHaveBeenCalled()
  })

  test("offers no Repair to someone who cannot edit", async () => {
    const { user } = renderBadge({ item: item({ can_edit: false }) })
    await user.click(await screen.findByRole("button", { name: /^Orders:/ }))
    expect(await screen.findByText(/^Checked/)).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Repair" })).toBeNull()
    expect(screen.queryByText(/Repair asks the agent/)).toBeNull()
  })
})
