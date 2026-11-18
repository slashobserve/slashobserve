import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ExportSheet } from "./export-sheet"
import { useExportStore } from "./export-store"
import { saveDashboardImage } from "./image"

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock("sonner", () => ({ toast }))
vi.mock("./image", () => ({ saveDashboardImage: vi.fn() }))

function Sheet({ state }: { state: string }) {
  return (
    <ExportSheet
      title="Checkout health"
      context="Ops, version 7"
      range="Time range: Last 6 hours"
      variables="env: prod"
      imageName="checkout-v7.png"
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
    expect(vi.mocked(saveDashboardImage).mock.calls[0][1]).toBe(
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
    expect(toast.error.mock.calls[0][0]).toMatch(/Print or save as PDF/)
    await waitFor(() => expect(useExportStore.getState().mode).toBeNull())
  })
})
