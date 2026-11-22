import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { HideCrumbContext } from "@/shell/crumbs"

import { RoleGate } from "./role-gate"

const session = vi.hoisted(() => ({ role: "viewer" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))

function renderGate(hideCrumb: boolean) {
  const setHidden = vi.fn()
  const view = render(
    <HideCrumbContext value={setHidden}>
      <RoleGate min="admin" hideCrumb={hideCrumb}>
        <p>the page</p>
      </RoleGate>
    </HideCrumbContext>
  )
  return { setHidden, view }
}

describe("RoleGate", () => {
  it("shows the page to a role that may open it and leaves the crumb alone", () => {
    session.role = "admin"
    const { setHidden } = renderGate(true)
    expect(screen.getByText("the page")).toBeInTheDocument()
    expect(setHidden).not.toHaveBeenCalled()
  })

  it("shows the note and keeps the crumb by default", () => {
    session.role = "viewer"
    const { setHidden } = renderGate(false)
    expect(
      screen.getByText("Only Admins can open this page.")
    ).toBeInTheDocument()
    expect(setHidden).not.toHaveBeenCalled()
  })

  it("hides the crumb trail while the note shows and restores it after", () => {
    session.role = "viewer"
    const { setHidden, view } = renderGate(true)
    expect(
      screen.getByText("Only Admins can open this page.")
    ).toBeInTheDocument()
    expect(setHidden).toHaveBeenLastCalledWith(true)
    view.unmount()
    expect(setHidden).toHaveBeenLastCalledWith(false)
  })
})
