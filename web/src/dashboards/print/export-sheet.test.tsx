import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useTheme } from "@/lib/theme"

import { ExportSheet } from "./export-sheet"
import { useExportStore } from "./export-store"
import { saveDashboardImage } from "./image"

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
}))

vi.mock("sonner", () => ({ toast }))
vi.mock("./image", () => ({ saveDashboardImage: vi.fn() }))

function Sheet({ state, timeoutMs }: { state: string; timeoutMs?: number }) {
  return (
    <ExportSheet
      title="Checkout health"
      context="Ops, version 7"
      range="Time range: Last 6 hours"
      variables="env: prod"
      imageName="checkout-v7.png"
      timeoutMs={timeoutMs}
    >
      <article data-panel-id="p1" data-state={state} />
    </ExportSheet>
  )
}

let print: ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.mocked(saveDashboardImage).mockReset()
  toast.success.mockReset()
  toast.error.mockReset()
  toast.warning.mockReset()
  print = vi.fn()
  vi.stubGlobal("print", print)
  useExportStore.getState().start("print")
})

afterEach(() => {
  useExportStore.getState().finish()
  vi.unstubAllGlobals()
})

describe("ExportSheet", () => {
  it("writes the header for paper", () => {
    render(<Sheet state="loading" />)
    expect(
      screen.getByRole("heading", { name: "Checkout health" })
    ).toBeInTheDocument()
    expect(screen.getByText("Ops, version 7")).toBeInTheDocument()
    expect(screen.getByText("Time range: Last 6 hours")).toBeInTheDocument()
    expect(screen.getByText("env: prod")).toBeInTheDocument()
    expect(screen.getByText(/^Printed /)).toBeInTheDocument()
  })

  it("prints once every panel has loaded and leaves print mode afterwards", async () => {
    render(<Sheet state="ready" />)
    await waitFor(() => expect(print).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    })
    expect(screen.getByRole("status")).toHaveTextContent("Ready to print.")
    act(() => {
      window.dispatchEvent(new Event("afterprint"))
    })
    expect(useExportStore.getState().mode).toBeNull()
  })

  it("does not print while a panel is loading and keeps the button disabled", () => {
    render(<Sheet state="loading" />)
    expect(print).not.toHaveBeenCalled()
    expect(
      screen.getByRole("button", { name: "Print or save as PDF" })
    ).toBeDisabled()
  })

  it("goes back to the dashboard from the button", async () => {
    render(<Sheet state="loading" />)
    await userEvent.click(
      screen.getByRole("button", { name: "Back to dashboard" })
    )
    expect(useExportStore.getState().mode).toBeNull()
  })

  it("draws the image instead of printing in image mode, then goes back", async () => {
    useExportStore.getState().start("image")
    render(<Sheet state="ready" />)
    await waitFor(() => expect(saveDashboardImage).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    })
    expect(vi.mocked(saveDashboardImage).mock.calls[0]?.[1]).toBe(
      "checkout-v7.png"
    )
    await waitFor(() => expect(useExportStore.getState().mode).toBeNull())
    expect(print).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith("Saved checkout-v7.png")
    expect(
      screen.queryByRole("button", { name: "Print or save as PDF" })
    ).toBeNull()
  })

  it("says what to try when the image cannot be drawn", async () => {
    useExportStore.getState().start("image")
    vi.mocked(saveDashboardImage).mockRejectedValue(new Error("too tall"))
    render(<Sheet state="ready" />)
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    })
    expect(toast.error.mock.calls[0]?.[0]).toMatch(/Print or save as PDF/)
    await waitFor(() => expect(useExportStore.getState().mode).toBeNull())
  })

  it("prints at once for a dashboard with no panel", async () => {
    render(
      <ExportSheet
        title="Empty"
        context="Ops, version 1"
        range="Time range: Last 1 hour"
        variables={null}
        imageName="empty-v1.png"
      >
        <p>No panels</p>
      </ExportSheet>
    )
    await waitFor(() => expect(print).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    })
  })

  it("does not print a half-loaded sheet and names the slow panel instead", async () => {
    render(<Sheet state="loading" timeoutMs={150} />)
    await waitFor(() => expect(toast.warning).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    })
    expect(toast.warning.mock.calls[0]?.[0]).toMatch(/Still loading: p1/)
    expect(print).not.toHaveBeenCalled()
    expect(screen.getByRole("status")).toHaveTextContent(
      "Some panels are still loading."
    )
    expect(
      screen.getByRole("button", { name: "Print or save as PDF" })
    ).toBeEnabled()
  })

  it("lets the person save the image anyway when a panel is slow", async () => {
    useExportStore.getState().start("image")
    render(<Sheet state="loading" timeoutMs={150} />)
    const anyway = await screen.findByRole(
      "button",
      { name: "Save the image anyway" },
      { timeout: 3000 }
    )
    expect(saveDashboardImage).not.toHaveBeenCalled()
    await userEvent.click(anyway)
    await waitFor(() => expect(saveDashboardImage).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(useExportStore.getState().mode).toBeNull())
  })

  it("prints in the light theme and gives the theme back afterwards", () => {
    const theme = renderHook(() => useTheme())
    act(() => theme.result.current.setPreference("dark"))
    const { unmount } = render(<Sheet state="loading" />)
    expect(theme.result.current.resolved).toBe("light")
    expect(document.documentElement).toHaveAttribute("data-export")
    unmount()
    expect(theme.result.current.resolved).toBe("dark")
    expect(document.documentElement).not.toHaveAttribute("data-export")
    act(() => theme.result.current.setPreference("system"))
  })

  it("keeps the person's theme for the image", () => {
    useExportStore.getState().start("image")
    const theme = renderHook(() => useTheme())
    act(() => theme.result.current.setPreference("dark"))
    render(<Sheet state="loading" />)
    expect(theme.result.current.resolved).toBe("dark")
    act(() => theme.result.current.setPreference("system"))
  })
})
