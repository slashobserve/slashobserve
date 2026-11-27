import { act, screen, waitFor } from "@testing-library/react"
import { useMutation } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Dashboard } from "@/spec"
import { renderWithProviders } from "@/test/render"

import type { SaveOutcome } from "../save-outcome"
import { useLayoutStore } from "./layout-store"
import { UnsavedBar } from "./unsaved-bar"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("@/spec", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/spec")>()),
  loadSpecValidator: async () => (spec: unknown) => ({ valid: true, spec }),
}))
vi.mock("../search", () => ({
  useDashboardSearch: () => ({}),
  useSetDashboardSearch: () => () => {},
}))
vi.mock("../queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../queries")>()),
  useAddVersion: () =>
    useMutation({
      mutationFn: (input: unknown) => calls.send(input) as Promise<SaveOutcome>,
    }),
}))

function specOf(x: number): Dashboard {
  return {
    specVersion: "0",
    title: "layout",
    time: { from: "now-1h", to: "now" },
    panels: [
      {
        id: "a",
        type: "stat" as const,
        title: "a",
        gridPos: { x, y: 0, w: 12, h: 4 },
        datasource: "prom",
        queries: [{ query: "up" }],
      },
    ],
  }
}

function state() {
  return useLayoutStore.getState()
}

afterEach(() => {
  state().end()
  vi.clearAllMocks()
})

describe("SaveLayoutDialog", () => {
  it("finishes the save when the saved version reaches the page before the response", async () => {
    let respond: (outcome: SaveOutcome) => void = () => {}
    calls.send.mockReturnValue(
      new Promise<SaveOutcome>((resolve) => {
        respond = resolve
      })
    )
    state().ensure("d1", 2, specOf(0))
    const session = state().session
    if (!session) throw new Error("no session")
    state().setLayout(
      session.layout.map((item) => ({ ...item, x: item.x + 1 }))
    )
    const { user } = renderWithProviders(<UnsavedBar dashboardId="d1" />)

    await user.click(screen.getByRole("button", { name: "Save version 3" }))
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(calls.send).toHaveBeenCalledTimes(1))

    act(() => state().ensure("d1", 3, specOf(1)))
    await waitFor(() =>
      expect(
        screen.queryByRole("region", { name: "Unsaved layout" })
      ).toBeNull()
    )
    await act(async () => {
      respond({
        kind: "saved",
        envelope: {} as never,
        version: 3,
        mergedFrom: null,
        warning: null,
      })
    })

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Saved version 3")
    )
    expect(state().session).toBeNull()
  })
})
