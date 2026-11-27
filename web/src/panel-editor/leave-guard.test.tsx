import { act, renderHook } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { useLeaveGuard } from "./leave-guard"

type BlockerOptions = {
  shouldBlockFn: (locations: { current: unknown; next: unknown }) => boolean
  enableBeforeUnload: () => boolean
}

const router = vi.hoisted(() => ({
  options: null as BlockerOptions | null,
}))

vi.mock("@tanstack/react-router", () => ({
  useBlocker: (options: BlockerOptions) => {
    router.options = options
    return { status: "idle" }
  },
}))

function blocksNavigation() {
  return router.options!.shouldBlockFn({ current: {}, next: {} })
}

function guard(onLeave: (kind: string) => void = () => {}) {
  return renderHook(() =>
    useLeaveGuard<string>({
      view: "editor",
      onViewChange: () => {},
      closeKind: "close",
      leaves: () => true,
      onLeave,
      trigger: () => null,
    })
  )
}

describe("useLeaveGuard", () => {
  it("blocks a navigation exactly while it reports unsaved edits", () => {
    const { result } = guard()
    expect(result.current.dirty).toBe(false)
    expect(blocksNavigation()).toBe(false)

    act(() => result.current.onDirtyChange(true))
    expect(result.current.dirty).toBe(true)
    expect(blocksNavigation()).toBe(true)
    expect(router.options!.enableBeforeUnload()).toBe(true)
  })

  it("reports no unsaved edits once a leave has run", () => {
    const { result } = guard()
    act(() => result.current.onDirtyChange(true))
    act(() => result.current.run("layout"))
    expect(result.current.dirty).toBe(blocksNavigation())
    expect(result.current.dirty).toBe(false)
  })

  it("lets the navigation a leave starts go through", () => {
    let blockedOwnLeave: boolean | null = null
    const { result } = guard(() => {
      blockedOwnLeave = blocksNavigation()
    })
    act(() => result.current.onDirtyChange(true))
    act(() => result.current.run("close"))
    expect(blockedOwnLeave).toBe(false)
  })
})
