import { screen } from "@testing-library/react"
import { describe, expect, test } from "vitest"

import { Button } from "@/components/ui/button"
import { renderWithProviders } from "@/test/render"

import type { HealthItem } from "./health-labels"
import { HealthyPopover } from "./health-popover"

const item = {
  id: "d1",
  title: "Checkout",
  url: "/d/d1/checkout",
  folder: { id: "f1", title: "Shop" },
  can_edit: true,
  checked_version: 4,
  checked_at: "2026-11-06T10:00:00Z",
  last_viewed_at: null,
  skipped: null,
  problems: [],
  repair: null,
  state: "healthy",
} as unknown as HealthItem

describe("HealthyPopover", () => {
  test("opens with when and on which version it was checked, and offers no repair", async () => {
    const { user } = renderWithProviders(
      <HealthyPopover
        item={item}
        align="start"
        trigger={<Button aria-haspopup="dialog" />}
      />,
      { route: { path: "/", url: "/" } }
    )
    const trigger = await screen.findByRole("button", { name: "Queries OK" })
    expect(trigger.querySelector("[data-status=ok]")).not.toBeNull()
    await user.click(trigger)
    const popover = await screen.findByRole("dialog")
    expect(popover.textContent).toContain("No problems found")
    expect(popover.textContent).toContain("Checked")
    expect(popover.textContent).toContain("on version 4")
    expect(screen.queryByRole("button", { name: /Repair/ })).toBeNull()
  })
})
