import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { RemoveDatasourceButton } from "./remove-datasource"
import type { DatasourceStatus } from "./queries"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("./admin-queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./admin-queries")>()),
  useDeleteDatasource: () =>
    useHeldMutation((uid: string) => calls.send(uid) as Promise<void>),
}))

const datasource = { uid: "prom", name: "Prometheus" } as DatasourceStatus

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("RemoveDatasourceButton", () => {
  it("reports the removal after the list dropped its row", async () => {
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <RemoveDatasourceButton datasource={datasource} label="Prometheus" />
      </HeldMutationHost>
    )

    await user.click(screen.getByRole("button", { name: "Remove Prometheus" }))
    await user.click(screen.getByRole("button", { name: "Remove" }))
    await waitFor(() => expect(calls.send).toHaveBeenCalledWith("prom"))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Removed Prometheus")
    )
  })
})
