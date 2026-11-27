import { act, renderHook } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { useInboxPanelStore } from "@/inbox/inbox-panel-store"

import { useSignOut } from "./use-sign-out"

const reset = vi.hoisted(() => vi.fn())
vi.mock("@/inbox/use-live-toasts", () => ({ resetLiveToasts: reset }))
vi.mock("@/lib/auth", () => ({
  logout: vi.fn(async () => {}),
  invalidateSession: vi.fn(),
}))
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => vi.fn() }))
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }))

beforeEach(() => reset.mockReset())

describe("useSignOut", () => {
  it("closes the Inbox panel and forgets the live toast cursor", async () => {
    useInboxPanelStore.setState({ open: true, view: "mentions" })
    const { result } = renderHook(() => useSignOut())
    await act(() => result.current.signOut())
    expect(useInboxPanelStore.getState().open).toBe(false)
    expect(useInboxPanelStore.getState().view).toBe("for_you")
    expect(reset).toHaveBeenCalledOnce()
  })

  it("removes the history older versions kept for every person", async () => {
    localStorage.setItem("slashobserve-prompt-history", '["old"]')
    localStorage.setItem("slashobserve-recent-dashboards", "[]")
    localStorage.setItem("slashobserve-prompt-history:u1", '["new"]')
    const { result } = renderHook(() => useSignOut())
    await act(() => result.current.signOut())
    expect(localStorage.getItem("slashobserve-prompt-history")).toBeNull()
    expect(localStorage.getItem("slashobserve-recent-dashboards")).toBeNull()
    expect(localStorage.getItem("slashobserve-prompt-history:u1")).toBe(
      '["new"]'
    )
  })
})
