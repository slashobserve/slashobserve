import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useInboxPanelStore } from "@/inbox/inbox-panel-store"

import { DockToggle } from "./dock-toggle"

afterEach(() => useInboxPanelStore.setState({ open: false }))

function renderToggle(pressed: boolean) {
  const onToggle = vi.fn()
  render(
    <DockToggle
      label="Discussion"
      count={0}
      pressed={pressed}
      onToggle={onToggle}
    />
  )
  return {
    onToggle,
    button: screen.getByRole("button", { name: "Discussion" }),
  }
}

describe("DockToggle", () => {
  it("toggles its panel while the Inbox is closed", async () => {
    const { onToggle, button } = renderToggle(true)
    expect(button).toHaveAttribute("aria-pressed", "true")

    await userEvent.click(button)

    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it("shows a panel hidden behind the Inbox as not pressed, and closes the Inbox to bring it back", async () => {
    useInboxPanelStore.setState({ open: true })
    const { onToggle, button } = renderToggle(true)
    expect(button).toHaveAttribute("aria-pressed", "false")

    await userEvent.click(button)

    expect(onToggle).not.toHaveBeenCalled()
    expect(useInboxPanelStore.getState().open).toBe(false)
    expect(button).toHaveAttribute("aria-pressed", "true")
  })

  it("opens a closed panel over the Inbox", async () => {
    useInboxPanelStore.setState({ open: true })
    const { onToggle, button } = renderToggle(false)

    await userEvent.click(button)

    expect(onToggle).toHaveBeenCalledTimes(1)
  })
})
