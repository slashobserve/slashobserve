import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { panelWidth } from "./card-width"
import { DockHeader } from "./dock-header"
import { DockPanel } from "./dock-panel"

const initialViewport = window.innerWidth

function setViewport(width: number) {
  window.innerWidth = width
  window.dispatchEvent(new Event("resize"))
}

function Panel({
  onClose = () => undefined,
  back,
  closeLabel = "Close panel",
}: {
  onClose?: () => void
  back?: () => void
  closeLabel?: string
}) {
  return (
    <DockPanel
      labelledBy="dock-title"
      width={420}
      bounds={panelWidth}
      onWidthChange={() => undefined}
      stacked={false}
    >
      <DockHeader
        headingId="dock-title"
        headingRef={null}
        title="Activity"
        description="Comments and saved versions."
        back={back}
        onClose={onClose}
        closeLabel={closeLabel}
      />
      <p>Body</p>
    </DockPanel>
  )
}

afterEach(() => setViewport(initialViewport))

describe("DockPanel", () => {
  it("renders an aside named by its heading with a resize handle", () => {
    setViewport(1440)
    render(<Panel />)
    const aside = screen.getByRole("complementary", { name: "Activity" })
    expect(aside).toHaveStyle({ "--dock-w": "420px" })
    expect(
      screen.getByRole("separator", { name: "Resize panel" })
    ).toHaveAttribute("aria-valuenow", "420")
    expect(screen.getByText("Comments and saved versions.")).toBeVisible()
  })

  it("gives the close button the name the caller passes", async () => {
    setViewport(1440)
    const onClose = vi.fn()
    render(<Panel onClose={onClose} closeLabel="Close" />)
    expect(screen.queryByRole("button", { name: "Close panel" })).toBeNull()
    await userEvent.click(screen.getByRole("button", { name: "Close" }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it("offers a way back to the activity only when asked", async () => {
    setViewport(1440)
    const back = vi.fn()
    const { rerender } = render(<Panel />)
    expect(
      screen.queryByRole("button", { name: "Back to activity" })
    ).toBeNull()
    rerender(<Panel back={back} />)
    await userEvent.click(
      screen.getByRole("button", { name: "Back to activity" })
    )
    expect(back).toHaveBeenCalledOnce()
  })

  it("has no close button without onClose", () => {
    setViewport(1440)
    render(
      <DockPanel
        labelledBy="chat-title"
        width={400}
        bounds={panelWidth}
        onWidthChange={() => undefined}
        stacked={false}
      >
        <DockHeader
          headingId="chat-title"
          headingRef={null}
          title="Chat"
          closeLabel="Close panel"
        />
      </DockPanel>
    )
    expect(screen.getByRole("complementary", { name: "Chat" })).toBeVisible()
    expect(screen.queryByRole("button", { name: "Close panel" })).toBeNull()
  })

  it("drops the resize handle when stacked under the content", () => {
    setViewport(1440)
    render(
      <DockPanel
        labelledBy="t"
        width={400}
        bounds={panelWidth}
        onWidthChange={() => undefined}
        stacked
      >
        <h2 id="t">Activity</h2>
      </DockPanel>
    )
    expect(screen.queryByRole("separator")).toBeNull()
  })

  it("becomes a bottom sheet on a phone whose grab bar collapses it to a peek and back", async () => {
    setViewport(390)
    render(<Panel />)
    const aside = screen.getByRole("complementary", { name: "Activity" })
    expect(screen.queryByRole("separator")).toBeNull()
    expect(aside).toHaveAttribute("data-sheet")
    expect(aside).not.toHaveAttribute("data-peek")

    await userEvent.click(
      screen.getByRole("button", { name: "Collapse panel" })
    )
    expect(aside).toHaveAttribute("data-peek")
    expect(screen.queryByRole("button", { name: "Collapse panel" })).toBeNull()

    await userEvent.click(screen.getByRole("button", { name: "Expand panel" }))
    expect(aside).not.toHaveAttribute("data-peek")
    expect(
      screen.getByRole("button", { name: "Collapse panel" })
    ).toBeInTheDocument()
  })

  it("has no grab bar on a wide screen", () => {
    setViewport(1440)
    render(<Panel />)
    expect(screen.queryByRole("button", { name: "Collapse panel" })).toBeNull()
    expect(
      screen.getByRole("complementary", { name: "Activity" })
    ).not.toHaveAttribute("data-sheet")
  })
})
