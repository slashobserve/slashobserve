import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useEffect, useRef, useState } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useOpenerFocus } from "./opener-focus"

function Harness({
  startOpen,
  fallback,
}: {
  startOpen: boolean
  fallback?: (() => HTMLElement | null) | undefined
}) {
  const [open, setOpen] = useState(startOpen)
  const [sharing, setSharing] = useState(false)
  return (
    <>
      <button
        onClick={() => {
          setOpen(true)
          setSharing(true)
        }}
      >
        Share
      </button>
      {open ? (
        <Pane
          sharing={sharing}
          fallback={fallback}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </>
  )
}

function Pane({
  sharing,
  fallback,
  onClose,
}: {
  sharing: boolean
  fallback?: (() => HTMLElement | null) | undefined
  onClose: () => void
}) {
  const card = useRef<HTMLElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const { remember, restore } = useOpenerFocus(card, fallback)
  useEffect(() => {
    remember()
    heading.current?.focus()
  }, [sharing, remember])
  return (
    <aside
      ref={card}
      onKeyDown={(event) => {
        if (event.key !== "Escape") return
        restore()
        onClose()
      }}
    >
      <h2 tabIndex={-1} ref={heading}>
        Pane
      </h2>
      <button onClick={remember}>Inside</button>
    </aside>
  )
}

beforeEach(() => {
  vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 0)
})

afterEach(() => vi.restoreAllMocks())

describe("useOpenerFocus", () => {
  it("returns focus to the button that opened the pane", async () => {
    const user = userEvent.setup()
    render(<Harness startOpen={false} />)
    await user.click(screen.getByRole("button", { name: "Share" }))
    expect(screen.getByRole("heading")).toHaveFocus()
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })

  it("returns focus to the button that switched an open pane to another mode", async () => {
    const user = userEvent.setup()
    render(<Harness startOpen />)
    await user.click(screen.getByRole("button", { name: "Share" }))
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })

  it("does not take an element inside the pane as the opener", async () => {
    const user = userEvent.setup()
    render(<Harness startOpen={false} />)
    await user.click(screen.getByRole("button", { name: "Share" }))
    await user.click(screen.getByRole("button", { name: "Inside" }))
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })

  it("does not take an element of a menu as the opener, because the menu is gone by the time the pane closes", async () => {
    const user = userEvent.setup()
    render(
      <>
        <button id="toggle">Toggle</button>
        <div role="menu">
          <button>Item</button>
        </div>
        <Harness
          startOpen={false}
          fallback={() => document.getElementById("toggle")}
        />
      </>
    )
    const item = screen.getByRole("button", { name: "Item" })
    item.focus()
    act(() => screen.getByRole("button", { name: "Share" }).click())
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Toggle" })).toHaveFocus()
  })

  it("focuses the fallback when the pane was open before anything opened it", async () => {
    const user = userEvent.setup()
    render(
      <>
        <button id="toggle">Toggle</button>
        <Harness startOpen fallback={() => document.getElementById("toggle")} />
      </>
    )
    await user.click(screen.getByRole("button", { name: "Inside" }))
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Toggle" })).toHaveFocus()
  })
})
