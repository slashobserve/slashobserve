import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { StrictMode, useRef, useState } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { pendingPaneHeading, useDockFocus } from "./dock-focus"

function Pane({
  takeFocus,
  onClose,
  holdClose,
  modeKey,
}: {
  takeFocus: () => boolean
  onClose: () => void
  holdClose?: () => boolean
  modeKey?: string
}) {
  const pane = useRef<HTMLElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  useDockFocus({
    pane,
    heading,
    opener: () => document.getElementById("opener"),
    onClose,
    holdClose,
    takeFocus,
    modeKey,
  })
  return (
    <aside ref={pane} aria-labelledby="heading">
      <h2 id="heading" ref={heading} tabIndex={-1}>
        Pane
      </h2>
      <button>Inside</button>
    </aside>
  )
}

function Page({
  startOpen = false,
  settle = true,
  holdClose,
}: {
  startOpen?: boolean
  settle?: boolean
  holdClose?: () => boolean
}) {
  const [open, setOpen] = useState(startOpen)
  const [mode, setMode] = useState("a")
  return (
    <>
      <button id="opener" onClick={() => setOpen(true)}>
        Open
      </button>
      <button onClick={() => setMode("b")}>Switch</button>
      {open ? (
        <Pane
          takeFocus={() => settle}
          modeKey={mode}
          holdClose={holdClose}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </>
  )
}

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe("useDockFocus", () => {
  it("moves focus to the heading when a person opens the pane", async () => {
    const user = userEvent.setup()
    render(<Page />)
    await user.click(screen.getByRole("button", { name: "Open" }))
    expect(screen.getByRole("heading")).toHaveFocus()
  })

  it("leaves focus alone for a pane that was open when the page loaded", () => {
    render(<Page startOpen settle={false} />)
    expect(document.body).toHaveFocus()
    expect(screen.getByRole("heading")).not.toHaveFocus()
  })

  it("leaves focus alone for a pane that was open at load when effects run twice", () => {
    render(
      <StrictMode>
        <Page startOpen settle={false} />
      </StrictMode>
    )
    expect(document.body).toHaveFocus()
  })

  it("moves focus to the heading when the pane switches mode with focus outside it", async () => {
    const user = userEvent.setup()
    render(<Page startOpen settle={false} />)
    await user.click(screen.getByRole("button", { name: "Switch" }))
    expect(screen.getByRole("heading")).toHaveFocus()
  })

  it("closes on Escape from inside the pane and returns focus to the opener", async () => {
    const user = userEvent.setup()
    render(<Page />)
    await user.click(screen.getByRole("button", { name: "Open" }))
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("heading")).toBeNull()
    expect(screen.getByRole("button", { name: "Open" })).toHaveFocus()
  })

  it("closes on Escape while focus is on the opener button", async () => {
    const user = userEvent.setup()
    render(<Page startOpen settle={false} />)
    screen.getByRole("button", { name: "Open" }).focus()
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("heading")).toBeNull()
    expect(screen.getByRole("button", { name: "Open" })).toHaveFocus()
  })

  it("ignores Escape pressed elsewhere on the page", async () => {
    const user = userEvent.setup()
    render(<Page startOpen settle={false} />)
    screen.getByRole("button", { name: "Switch" }).focus()
    await user.keyboard("{Escape}")
    expect(screen.getByRole("heading")).toBeInTheDocument()
  })

  it("keeps the pane open and the focus where it is while a close is held", async () => {
    const user = userEvent.setup()
    const hold = vi.fn(() => true)
    render(<Page holdClose={hold} />)
    await user.click(screen.getByRole("button", { name: "Open" }))
    await user.keyboard("{Escape}")
    expect(hold).toHaveBeenCalledOnce()
    expect(screen.getByRole("heading")).toHaveFocus()
  })
})

describe("pendingPaneHeading", () => {
  it("keeps naming it when effects run twice", async () => {
    const user = userEvent.setup()
    render(
      <StrictMode>
        <Page />
      </StrictMode>
    )
    await user.click(screen.getByRole("button", { name: "Open" }))
    expect(pendingPaneHeading()).toBe(screen.getByRole("heading"))
  })

  it("names the heading of a pane a person just opened, so a closing menu or sheet hands focus to it", async () => {
    const user = userEvent.setup()
    render(<Page />)
    await user.click(screen.getByRole("button", { name: "Open" }))
    expect(pendingPaneHeading()).toBe(screen.getByRole("heading"))
  })

  it("stops naming it after a moment", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(<Page />)
    await user.click(screen.getByRole("button", { name: "Open" }))
    act(() => vi.advanceTimersByTime(2500))
    expect(pendingPaneHeading()).toBeNull()
    vi.useRealTimers()
  })

  it("is null for a pane that was open when the page loaded", () => {
    render(<Page startOpen settle={false} />)
    expect(pendingPaneHeading()).toBeNull()
  })
})
