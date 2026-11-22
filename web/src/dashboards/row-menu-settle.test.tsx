import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { components } from "@/api/schema.gen"
import { folderKeys } from "@/api/keys"
import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { DashboardRowMenu } from "./row-menu"

type Summary = components["schemas"]["DashboardListItem"]

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  move: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { role: "admin" } }),
}))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  useMoveDashboard: () =>
    useHeldMutation((input: { id: string; folderId: string }) =>
      calls.move(input)
    ),
}))

const shopId = "22222222-2222-4222-8222-222222222222"
const opsId = "33333333-3333-4333-8333-333333333333"

function folder(id: string, title: string) {
  return {
    id,
    title,
    general: false,
    personal: false,
    permissions: { comment: true, edit: true, manage: true, create: true },
  }
}

const dashboard = {
  id: "44444444-4444-4444-8444-444444444444",
  title: "Audit overview",
  latest_version: 2,
  folder: { id: shopId, title: "Shop", general: false, personal: false },
  permissions: { comment: true, edit: true, manage: true, create: true },
  provisioned: false,
} as Summary

beforeEach(() => {
  resetHeldMutation()
  calls.move.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("DashboardRowMenu", () => {
  it("reports a move after the list dropped its row", async () => {
    const queryClient = testQueryClient()
    queryClient.setQueryData(folderKeys.list(), [
      folder(shopId, "Shop"),
      folder(opsId, "Ops"),
    ])
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <DashboardRowMenu dashboard={dashboard} />
      </HeldMutationHost>,
      { queryClient }
    )

    await user.click(
      screen.getByRole("button", { name: "More for Audit overview" })
    )
    await user.click(await screen.findByRole("menuitemradio", { name: "Ops" }))
    await waitFor(() =>
      expect(calls.move).toHaveBeenCalledWith({
        id: dashboard.id,
        folderId: opsId,
      })
    )
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Moved to Ops")
    )
  })
})
