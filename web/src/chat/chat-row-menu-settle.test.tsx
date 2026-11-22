import { createRef } from "react"
import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import {
  HeldMutationHost,
  resetHeldMutation,
  unmountHostThenSettle,
  useHeldMutation,
} from "@/test/held-mutation"
import { renderWithProviders } from "@/test/render"

import { ChatRowMenu } from "./chat-row-menu"

const calls = vi.hoisted(() => ({
  navigate: vi.fn(),
  deleteTurns: vi.fn(),
}))

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => calls.navigate,
}))
vi.mock("@/observe/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/observe/queries")>()),
  useDeleteChatTurns: () =>
    useHeldMutation(() => calls.deleteTurns() as Promise<void>),
}))

const request = { id: "r1", title: "Checkout latency", pinned_at: null }

beforeEach(() => {
  resetHeldMutation()
  calls.deleteTurns.mockResolvedValue(undefined)
})

afterEach(() => vi.clearAllMocks())

describe("ChatRowMenu", () => {
  it("leaves a deleted current chat after the list dropped its row", async () => {
    const afterDelete = createRef<HTMLElement>()
    const { user } = renderWithProviders(
      <HeldMutationHost>
        <ChatRowMenu
          request={request}
          turns={[]}
          current
          onRename={() => {}}
          afterDelete={afterDelete}
        />
      </HeldMutationHost>
    )

    await user.click(
      screen.getByRole("button", { name: "Chat actions: Checkout latency" })
    )
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete chat",
      })
    )
    await waitFor(() => expect(calls.deleteTurns).toHaveBeenCalledTimes(1))
    await unmountHostThenSettle()

    await waitFor(() =>
      expect(calls.navigate).toHaveBeenCalledWith({ to: "/ask" })
    )
  })
})
