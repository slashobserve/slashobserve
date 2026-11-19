import { renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useExportStore } from "./print/export-store"
import { useInView } from "./use-in-view"

class IdleObserver {
  observe() {}
  disconnect() {}
}

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", IdleObserver)
})

afterEach(() => {
  useExportStore.getState().finish()
  vi.unstubAllGlobals()
})

function element() {
  return { current: document.createElement("div") }
}

describe("useInView", () => {
  it("has seen nothing until the element scrolls into view", () => {
    const { result } = renderHook(() => useInView(element()))
    expect(result.current).toEqual({ seen: false, inView: false })
  })

  it("counts every panel as seen while a dashboard is being exported", () => {
    useExportStore.getState().start("image")
    const { result } = renderHook(() => useInView(element()))
    expect(result.current).toEqual({ seen: true, inView: true })
  })
})
