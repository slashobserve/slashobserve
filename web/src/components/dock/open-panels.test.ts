import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import {
  panelLeft,
  panelShown,
  resetOpenPanels,
  useDockPanelShown,
} from "./open-panels"

afterEach(() => act(() => resetOpenPanels()))

describe("useDockPanelShown", () => {
  it("follows the panels that are shown", () => {
    const { result } = renderHook(() => useDockPanelShown())
    expect(result.current).toBe(false)

    act(() => {
      panelShown("side-panel:a")
    })
    expect(result.current).toBe(true)

    act(() => {
      panelShown("side-panel:b")
      panelLeft("side-panel:a", false)
    })
    expect(result.current).toBe(true)

    act(() => panelLeft("side-panel:b", false))
    expect(result.current).toBe(false)
  })
})
