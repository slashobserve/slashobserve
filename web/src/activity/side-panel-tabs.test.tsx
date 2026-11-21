import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import type { SidePanelTab } from "@/dashboards/search"

import { SidePanelTabs } from "./side-panel-tabs"

function renderTabs(tab: SidePanelTab) {
  const onTabChange = vi.fn()
  const onClose = vi.fn()
  render(
    <SidePanelTabs
      headingId="side-panel-heading"
      headingRef={null}
      tab={tab}
      onTabChange={onTabChange}
      onClose={onClose}
    >
      <p>{tab} body</p>
    </SidePanelTabs>
  )
  return { onTabChange, onClose, user: userEvent.setup() }
}

describe("SidePanelTabs", () => {
  it("marks the open tab and names the panel after it", () => {
    renderTabs("activity")
    expect(screen.getByRole("tab", { name: "Activity" })).toHaveAttribute(
      "aria-selected",
      "true"
    )
    expect(screen.getByRole("tab", { name: "Chat" })).toHaveAttribute(
      "aria-selected",
      "false"
    )
    expect(
      screen.getByRole("heading", { name: "Activity" })
    ).toBeInTheDocument()
    expect(screen.getByText("activity body")).toBeInTheDocument()
  })

  it("asks for the other tab and for closing", async () => {
    const { user, onTabChange, onClose } = renderTabs("activity")
    await user.click(screen.getByRole("tab", { name: "Chat" }))
    expect(onTabChange).toHaveBeenCalledWith("chat")
    await user.click(screen.getByRole("button", { name: "Close panel" }))
    expect(onClose).toHaveBeenCalled()
  })
})
