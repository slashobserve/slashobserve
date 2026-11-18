import { renderHook, act } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { holdLightTheme, useTheme } from "./theme"

describe("holdLightTheme", () => {
  it("shows the light theme until released, whatever the person chose", () => {
    const { result } = renderHook(() => useTheme())
    act(() => result.current.setPreference("dark"))
    expect(result.current.resolved).toBe("dark")
    expect(document.documentElement.classList.contains("dark")).toBe(true)

    let release = () => {}
    act(() => {
      release = holdLightTheme()
    })
    expect(result.current.resolved).toBe("light")
    expect(document.documentElement.classList.contains("dark")).toBe(false)
    expect(result.current.preference).toBe("dark")

    act(() => release())
    expect(result.current.resolved).toBe("dark")
    expect(document.documentElement.classList.contains("dark")).toBe(true)
    act(() => result.current.setPreference("system"))
  })

  it("keeps a preference chosen meanwhile for after the release", () => {
    const { result } = renderHook(() => useTheme())
    act(() => result.current.setPreference("light"))
    let release = () => {}
    act(() => {
      release = holdLightTheme()
      result.current.setPreference("dark")
    })
    expect(result.current.resolved).toBe("light")
    act(() => release())
    expect(result.current.resolved).toBe("dark")
    act(() => result.current.setPreference("system"))
  })
})
