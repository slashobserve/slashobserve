import { act, renderHook } from "@testing-library/react"
import { expect, it } from "vitest"

import { useUserScroll, userScrollWindowMs } from "./user-scroll"

it("reports no user input before anything happens", () => {
  const { result } = renderHook(() => useUserScroll(() => 0))
  expect(result.current.recent()).toBe(false)
})

it("treats a scroll within the window after a wheel as the user's", () => {
  let clock = 1000
  const { result } = renderHook(() => useUserScroll(() => clock))
  result.current.handlers.onWheelCapture()
  clock += userScrollWindowMs
  expect(result.current.recent()).toBe(true)
  clock += 1
  expect(result.current.recent()).toBe(false)
})

it("keeps counting a scrollbar drag until the pointer is released", () => {
  let clock = 0
  const { result } = renderHook(() => useUserScroll(() => clock))
  result.current.handlers.onPointerDownCapture()
  clock += 10 * userScrollWindowMs
  expect(result.current.recent()).toBe(true)
  act(() => {
    document.dispatchEvent(new Event("pointerup"))
  })
  expect(result.current.recent()).toBe(false)
})
