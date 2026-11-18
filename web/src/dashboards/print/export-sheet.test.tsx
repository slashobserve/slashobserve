import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useExportStore } from "./export-store"
import { ExportSheet } from "./export-sheet"

function Sheet({ state }: { state: string }) {
  return (
    <ExportSheet
      title="Checkout health"
      context="Ops, version 7"
      range="Time range: Last 6 hours"
      variables="env: prod"
    >
      <article data-panel-id="p1" data-state={state} />
    </ExportSheet>
  )
}

let print: ReturnType<typeof vi.fn>

beforeEach(() => {
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
})
