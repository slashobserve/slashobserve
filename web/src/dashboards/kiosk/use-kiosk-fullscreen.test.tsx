import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"

import { requestFullscreen, useKioskFullscreen } from "./use-kiosk-fullscreen"

let element: Element | null = null

function setFullscreen(next: Element | null) {
  element = next
  document.dispatchEvent(new Event("fullscreenchange"))
}

function later(change: () => void): Promise<void> {
  return Promise.resolve().then(change)
}

const exitFullscreen = vi.fn(() => later(() => setFullscreen(null)))
const enterFullscreen = vi.fn(() =>
  later(() => setFullscreen(document.documentElement))
)

beforeEach(() => {
  element = null
  Object.defineProperty(document, "fullscreenElement", {
    configurable: true,
    get: () => element,
  })
  Object.defineProperty(document, "fullscreenEnabled", {
    configurable: true,
    value: true,
  })
  Object.defineProperty(document, "exitFullscreen", {
    configurable: true,
    value: exitFullscreen,
  })
  Object.defineProperty(document.documentElement, "requestFullscreen", {
    configurable: true,
    value: enterFullscreen,
  })
})

afterEach(() => {
  for (const key of [
    "fullscreenElement",
    "fullscreenEnabled",
    "exitFullscreen",
  ]) {
    Reflect.deleteProperty(document, key)
  }
  Reflect.deleteProperty(document.documentElement, "requestFullscreen")
  vi.clearAllMocks()
})

function renderKiosk(kiosk: boolean, leaveKiosk = vi.fn()) {
  const view = renderHook(
    ({ kiosk }) => useKioskFullscreen(kiosk, leaveKiosk),
    { initialProps: { kiosk }, reactStrictMode: true }
  )
  return { ...view, leaveKiosk }
}

async function settle() {
  await act(async () => {
    await Promise.resolve()
  })
}

describe("useKioskFullscreen", () => {
  test("leaves TV mode once when the browser leaves full screen", async () => {
    element = document.documentElement
    const { leaveKiosk } = renderKiosk(true)
    act(() => setFullscreen(null))
    await settle()
    expect(leaveKiosk).toHaveBeenCalledTimes(1)
  })

  test("exits full screen when TV mode ends and does not leave twice", async () => {
    element = document.documentElement
    const { result, rerender, leaveKiosk } = renderKiosk(true)
    act(() => result.current.leave())
    rerender({ kiosk: false })
    await settle()
    expect(exitFullscreen).toHaveBeenCalledTimes(1)
    expect(element).toBeNull()
    expect(leaveKiosk).toHaveBeenCalledTimes(1)
  })

  test("exits full screen when TV mode ends another way", async () => {
    element = document.documentElement
    const { rerender, leaveKiosk } = renderKiosk(true)
    rerender({ kiosk: false })
    await settle()
    expect(exitFullscreen).toHaveBeenCalledTimes(1)
    expect(leaveKiosk).not.toHaveBeenCalled()
  })

  test("leaves once when full screen ends before TV mode does", async () => {
    element = document.documentElement
    const { result, leaveKiosk } = renderKiosk(true)
    act(() => result.current.leave())
    act(() => setFullscreen(null))
    expect(leaveKiosk).toHaveBeenCalledTimes(1)
  })

  test("keeps TV mode opened from a link and offers full screen", async () => {
    const { result, leaveKiosk } = renderKiosk(true)
    await settle()
    expect(result.current.offerFullscreen).toBe(true)
    expect(leaveKiosk).not.toHaveBeenCalled()
    expect(exitFullscreen).not.toHaveBeenCalled()
    act(() => requestFullscreen())
    await settle()
    expect(enterFullscreen).toHaveBeenCalledTimes(1)
    expect(result.current.offerFullscreen).toBe(false)
    expect(leaveKiosk).not.toHaveBeenCalled()
  })

  test("offers nothing outside TV mode", () => {
    const { result } = renderKiosk(false)
    expect(result.current.offerFullscreen).toBe(false)
  })

  test("keeps full screen and TV mode across a remount, as the folder cycle does", async () => {
    element = document.documentElement
    const first = renderKiosk(true)
    first.unmount()
    const second = renderKiosk(true)
    await settle()
    expect(exitFullscreen).not.toHaveBeenCalled()
    expect(element).toBe(document.documentElement)
    act(() => setFullscreen(null))
    expect(first.leaveKiosk).not.toHaveBeenCalled()
    expect(second.leaveKiosk).toHaveBeenCalledTimes(1)
  })

  test("ignores the leave call outside TV mode", () => {
    const { result, leaveKiosk } = renderKiosk(false)
    act(() => result.current.leave())
    expect(leaveKiosk).not.toHaveBeenCalled()
  })
})

describe("requestFullscreen", () => {
  test("stays quiet when the browser refuses full screen", async () => {
    enterFullscreen.mockImplementationOnce(() =>
      Promise.reject(new TypeError("Permissions check failed"))
    )
    requestFullscreen()
    await Promise.resolve()
    expect(enterFullscreen).toHaveBeenCalledTimes(1)
    expect(element).toBeNull()
  })

  test("asks nothing of a browser without full screen", () => {
    Object.defineProperty(document, "fullscreenEnabled", {
      configurable: true,
      value: false,
    })
    requestFullscreen()
    expect(enterFullscreen).not.toHaveBeenCalled()
  })
})
