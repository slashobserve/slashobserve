import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { StrictMode, useRef, useState } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import {
  announcePaneOpening,
  keepFocusThroughPaneSwitch,
  pendingPaneHeading,
  useDockFocus,
} from "./dock-focus"

function Pane({
  onClose,
  holdClose,
  modeKey,
  active,
}: {
  onClose: () => void
  holdClose?: () => boolean
  modeKey?: string
  active?: boolean
}) {
  const pane = useRef<HTMLElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  useDockFocus({
    pane,
    heading,
    opener: () => document.getElementById("opener"),
    onClose,
    holdClose,
    modeKey,
    active,
  })
  return (
    <aside ref={pane} aria-labelledby="heading" hidden={active === false}>
      <h2 id="heading" ref={heading} tabIndex={-1}>
        Pane
      </h2>
      <button>Inside</button>
    </aside>
  )
}

function Page({
  startOpen = false,
  holdClose,
}: {
  startOpen?: boolean
  holdClose?: () => boolean
}) {
  const [open, setOpen] = useState(startOpen)
  const [mode, setMode] = useState("a")
  const [active, setActive] = useState(true)
  return (
    <>
      <button
        id="opener"
        onClick={() => {
          announcePaneOpening()
          setOpen(true)
        }}
      >
        Open
      </button>
      <button onClick={() => setOpen((current) => !current)}>Remount</button>
      <button onClick={() => setMode("b")}>Switch</button>
      <button
        onClick={() => {
          keepFocusThroughPaneSwitch()
          setMode("c")
        }}
      >
        Switch quietly
      </button>
      <button onClick={() => setActive((current) => !current)}>Hide</button>
      {open ? (
        <Pane
          active={active}
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
    render(<Page startOpen />)
    expect(document.body).toHaveFocus()
    expect(screen.getByRole("heading")).not.toHaveFocus()
  })

  it("leaves focus alone for a pane that was open at load when effects run twice", () => {
    render(
      <StrictMode>
        <Page startOpen />
      </StrictMode>
    )
    expect(document.body).toHaveFocus()
  })

  it("does not move focus when the pane remounts without a person opening it, as when a save ends layout editing", async () => {
    const user = userEvent.setup()
    render(<Page startOpen />)
    await user.click(screen.getByRole("button", { name: "Remount" }))
    expect(screen.queryByRole("heading")).toBeNull()
    screen.getByRole("button", { name: "Switch" }).focus()
    await user.click(screen.getByRole("button", { name: "Remount" }))
    expect(screen.getByRole("heading")).toBeInTheDocument()
    expect(screen.getByRole("heading")).not.toHaveFocus()
    expect(pendingPaneHeading()).toBeNull()
  })

  it("does not move focus when a pane a person opened earlier is shown again after being hidden, as when a layout save ends", async () => {
    const user = userEvent.setup()
    render(<Page />)
    await user.click(screen.getByRole("button", { name: "Open" }))
    expect(screen.getByRole("heading", { hidden: true })).toHaveFocus()
    const hide = screen.getByRole("button", { name: "Hide" })
    await user.click(hide)
    await user.click(hide)
    expect(screen.getByRole("heading")).not.toHaveFocus()
    expect(hide).toHaveFocus()
  })

  it("moves focus to the heading when the pane switches mode with focus outside it", async () => {
    const user = userEvent.setup()
    render(<Page startOpen />)
    await user.click(screen.getByRole("button", { name: "Switch" }))
    expect(screen.getByRole("heading")).toHaveFocus()
  })

  it("leaves focus where it is when the switch keeps it, as when a canvas ask moves the pane to its chat", async () => {
    const user = userEvent.setup()
    render(<Page startOpen />)
    const quiet = screen.getByRole("button", { name: "Switch quietly" })
    await user.click(quiet)
    expect(quiet).toHaveFocus()
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
    render(<Page startOpen />)
    screen.getByRole("button", { name: "Open" }).focus()
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("heading")).toBeNull()
    expect(screen.getByRole("button", { name: "Open" })).toHaveFocus()
  })

  it("ignores Escape pressed elsewhere on the page", async () => {
    const user = userEvent.setup()
    render(<Page startOpen />)
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
    render(<Page startOpen />)
    expect(pendingPaneHeading()).toBeNull()
  })
})
