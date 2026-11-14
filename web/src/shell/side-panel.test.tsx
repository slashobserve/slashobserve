import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useRef, useState } from "react"
import { beforeEach, describe, expect, it } from "vitest"

import { PanelHostContext, SidePanel } from "./side-panel"

function Harness() {
  const [open, setOpen] = useState(false)
  const opener = useRef<HTMLButtonElement>(null)
  return (
    <>
      <button ref={opener} onClick={() => setOpen(true)}>
        Open panel
      </button>
      <button>Elsewhere</button>
      {open ? (
        <SidePanel title="Add" trigger={opener} onClose={() => setOpen(false)}>
          <button onClick={() => setOpen(false)}>Done</button>
        </SidePanel>
      ) : null}
    </>
  )
}

function renderHarness() {
  const host = document.createElement("div")
  document.body.append(host)
  const user = userEvent.setup()
  render(
    <PanelHostContext.Provider value={host}>
      <Harness />
    </PanelHostContext.Provider>
  )
  return user
}

beforeEach(() => {
  document.body.replaceChildren()
})

describe("SidePanel focus on close", () => {
  it("returns focus to the button that opened it when focus was inside the panel", async () => {
    const user = renderHarness()
    await user.click(screen.getByRole("button", { name: "Open panel" }))
    await user.click(screen.getByRole("button", { name: "Done" }))
    expect(screen.getByRole("button", { name: "Open panel" })).toHaveFocus()
  })

  it("leaves focus where it moved while the panel was open", async () => {
    const user = renderHarness()
    await user.click(screen.getByRole("button", { name: "Open panel" }))
    screen.getByRole("button", { name: "Elsewhere" }).focus()
    fireEvent.click(screen.getByRole("button", { name: "Close" }))
    expect(screen.getByRole("button", { name: "Elsewhere" })).toHaveFocus()
  })
})
