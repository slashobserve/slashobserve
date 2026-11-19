import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useEffect, useRef, useState } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useOpenerFocus } from "./opener-focus"

function Harness({ startOpen }: { startOpen: boolean }) {
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
      {open ? <Pane sharing={sharing} onClose={() => setOpen(false)} /> : null}
    </>
  )
}

function Pane({ sharing, onClose }: { sharing: boolean; onClose: () => void }) {
  const card = useRef<HTMLElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const { remember, restore } = useOpenerFocus(card)
  useEffect(() => {
    remember()
    heading.current?.focus()
  }, [sharing, remember])
  return (
    <aside
      ref={card}
      onKeyDown={(event) => {
        if (event.key !== "Escape") return
        onClose()
        restore()
      }}
    >
      <h2 tabIndex={-1} ref={heading}>
        Pane
      </h2>
      <button onClick={remember}>Inside</button>
    </aside>
  )
}

afterEach(() => vi.restoreAllMocks())

describe("useOpenerFocus", () => {
  it("returns focus to the button that opened the pane", async () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((run) => {
      run(0)
      return 0
    })
    const user = userEvent.setup()
    render(<Harness startOpen={false} />)
    await user.click(screen.getByRole("button", { name: "Share" }))
    expect(screen.getByRole("heading")).toHaveFocus()
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })

  it("returns focus to the button that switched an open pane to another mode", async () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((run) => {
      run(0)
      return 0
    })
    const user = userEvent.setup()
    render(<Harness startOpen />)
    await user.click(screen.getByRole("button", { name: "Share" }))
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })

  it("does not take an element inside the pane as the opener", async () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((run) => {
      run(0)
      return 0
    })
    const user = userEvent.setup()
    render(<Harness startOpen={false} />)
    await user.click(screen.getByRole("button", { name: "Share" }))
    await user.click(screen.getByRole("button", { name: "Inside" }))
    await act(() => user.keyboard("{Escape}"))
    expect(screen.getByRole("button", { name: "Share" })).toHaveFocus()
  })
})
