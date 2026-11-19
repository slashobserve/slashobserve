import { afterEach, describe, expect, it, vi } from "vitest"

import { holdSheet, useExportStore } from "./export-store"

afterEach(() => {
  useExportStore.getState().finish()
  vi.useRealTimers()
})

describe("useExportStore", () => {
  it("starts and finishes an export", () => {
    useExportStore.getState().start("print")
    expect(useExportStore.getState().mode).toBe("print")
    useExportStore.getState().finish()
    expect(useExportStore.getState().mode).toBeNull()
  })
})

describe("holdSheet", () => {
  it("finishes the export when the sheet goes away", () => {
    vi.useFakeTimers()
    useExportStore.getState().start("print")
    holdSheet()()
    vi.runAllTimers()
    expect(useExportStore.getState().mode).toBeNull()
  })

  it("keeps the export when the sheet is mounted again at once, as in strict mode", () => {
    vi.useFakeTimers()
    useExportStore.getState().start("print")
    const release = holdSheet()
    release()
    const release2 = holdSheet()
    vi.runAllTimers()
    expect(useExportStore.getState().mode).toBe("print")
    release2()
    vi.runAllTimers()
    expect(useExportStore.getState().mode).toBeNull()
  })
})
