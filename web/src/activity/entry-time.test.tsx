import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { formatDateTime } from "@/lib/time"

import { EntryTime } from "./entry-time"

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 10, 15, 16, 0, 0))
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe("EntryTime", () => {
  it("shows the local clock time of an older entry", () => {
    const iso = new Date(2026, 10, 15, 14, 32, 5).toISOString()
    render(<EntryTime iso={iso} />)
    expect(screen.getByText("14:32")).toBeTruthy()
  })

  it("pads the hour and minute", () => {
    const iso = new Date(2026, 10, 14, 9, 5, 0).toISOString()
    render(<EntryTime iso={iso} />)
    expect(screen.getByText("09:05")).toBeTruthy()
  })

  it("says Just now for an entry under a minute old", () => {
    const iso = new Date(2026, 10, 15, 15, 59, 50).toISOString()
    render(<EntryTime iso={iso} />)
    expect(screen.getByText("Just now")).toBeTruthy()
  })

  it("shows the clock time once an entry is a minute old", () => {
    const iso = new Date(2026, 10, 15, 15, 59, 0).toISOString()
    render(<EntryTime iso={iso} />)
    expect(screen.getByText("15:59")).toBeTruthy()
  })

  it("carries the full date and time as its title", () => {
    const iso = new Date(2026, 10, 15, 14, 32, 5).toISOString()
    render(<EntryTime iso={iso} />)
    const time = screen.getByText("14:32")
    expect(time.getAttribute("title")).toBe(formatDateTime(iso))
    expect(time.getAttribute("datetime")).toBe(iso)
  })
})
