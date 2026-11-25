import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { InputMode } from "@/ask/input-mode"

import { useLeaveAskOnSavedOpen, useSavedOpens } from "./saved-open"

vi.mock("@tanstack/react-router", () => ({ useNavigate: () => vi.fn() }))

afterEach(() => {
  useSavedOpens.setState({ count: 0 })
})

function mounted(mode: InputMode) {
  const setMode = vi.fn()
  const view = renderHook(
    ({ current }) => useLeaveAskOnSavedOpen(current, setMode),
    { initialProps: { current: mode } }
  )
  return { setMode, view }
}

describe("useLeaveAskOnSavedOpen", () => {
  it("leaves Ask for the query without remembering it each time a saved search opens", () => {
    const { setMode } = mounted("ask")
    act(() => useSavedOpens.getState().bump())
    expect(setMode).toHaveBeenCalledWith("code", false)
    act(() => useSavedOpens.getState().bump())
    expect(setMode).toHaveBeenCalledTimes(2)
  })

  it.each(["code", "builder"] as const)("keeps %s", (mode) => {
    const { setMode } = mounted(mode)
    act(() => useSavedOpens.getState().bump())
    expect(setMode).not.toHaveBeenCalled()
  })

  it("ignores opens from before it mounted and rerenders without one", () => {
    useSavedOpens.getState().bump()
    const { setMode, view } = mounted("ask")
    view.rerender({ current: "ask" })
    expect(setMode).not.toHaveBeenCalled()
  })
})
