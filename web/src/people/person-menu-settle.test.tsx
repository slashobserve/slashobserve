import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { PersonMenu } from "./person-menu"
import type { User } from "./queries"

const calls = vi.hoisted(() => ({
  toast: { success: vi.fn(), error: vi.fn() },
  send: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: calls.toast }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "me" } }),
}))
vi.mock("./person-lifecycle", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./person-lifecycle")>()),
  usePersonAction: () =>
    useHeldMutation((input: { id: string; action: string }) =>
      calls.send(input)
    ),
}))

const user = {
  id: "u1",
  name: "Ada Lovelace",
  email: "ada@example.com",
  key_count: 0,
  personal_items: null,
  deactivated_at: "2026-11-01T00:00:00Z",
} as User

beforeEach(() => {
  resetHeldMutation()
  calls.send.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("PersonMenu", () => {
  it("reports a removal after the list dropped the person", async () => {
    const { user: actor } = renderWithProviders(
      <HeldMutationHost>
        <PersonMenu user={user} />
      </HeldMutationHost>
    )

    await actor.click(
      screen.getByRole("button", { name: "More for Ada Lovelace" })
    )
    await actor.click(await screen.findByRole("menuitem", { name: "Remove" }))
    await actor.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Remove",
      })
    )
    await waitFor(() =>
      expect(calls.send).toHaveBeenCalledWith({ id: "u1", action: "remove" })
    )
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.toast.success).toHaveBeenCalledWith("Removed Ada Lovelace")
    )
  })
})
