import { createRef } from "react"
import { screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { usersQuery, type User } from "@/people/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { KeysTable } from "./keys-table"
import type { ServiceAccount, ServiceAccountKey } from "./queries"

const account = {
  id: "a1",
  name: "ci-bot",
  description: "",
  role: "viewer",
  provisioned: false,
} as ServiceAccount

const provisioned = {
  ...account,
  id: "a2",
  name: "from-file",
  provisioned: true,
} as ServiceAccount

function key(id: string, name: string, createdBy: string | null) {
  return {
    id,
    name,
    display_prefix: `sok_${id}`,
    created_by: createdBy,
    last_used_at: null,
    expires_at: null,
    state: "active",
  } as unknown as ServiceAccountKey
}

const state = vi.hoisted(() => ({ loadingMore: false }))

vi.mock("./use-all-keys", () => ({
  useAllKeys: () => ({
    status: "success",
    error: null,
    accounts: [account, provisioned],
    failedAccounts: [],
    loadingMore: state.loadingMore,
    fetching: false,
    refetch: () => {},
    rows: [
      { key: key("k1", "deploy", "u1"), account },
      { key: key("k2", "seed", null), account: provisioned },
    ],
  }),
}))
vi.mock("./revoke-key-button", () => ({
  RevokeKeyButton: ({ serviceKey }: { serviceKey: ServiceAccountKey }) => (
    <button type="button">Revoke {serviceKey.name}</button>
  ),
}))

function renderTable() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(usersQuery().queryKey, [
    { id: "u1", name: "Ada Lovelace" } as User,
  ])
  const revokeFocus = createRef<HTMLElement>()
  return renderWithProviders(
    <KeysTable onCreate={() => {}} revokeFocus={revokeFocus} />,
    { queryClient }
  )
}

describe("KeysTable", () => {
  it("draws a row per key, tagged with the key's id, with who created it", async () => {
    const { container } = renderTable()
    await screen.findByText("sok_k1…")

    const deploy = container.querySelector<HTMLElement>(
      '[data-key-id="k1"]'
    ) as HTMLElement
    expect(deploy).toHaveTextContent("ci-bot")
    expect(within(deploy).getByText("sok_k1…")).toBeInTheDocument()
    expect(within(deploy).getByText("Ada Lovelace")).toBeInTheDocument()
    expect(within(deploy).getByText("Never")).toBeInTheDocument()

    const seed = container.querySelector<HTMLElement>(
      '[data-key-id="k2"]'
    ) as HTMLElement
    expect(seed).toHaveTextContent("from-file - from file")
    expect(within(seed).getByText("Service accounts file")).toBeInTheDocument()
  })

  it("keeps a row's revoke button out of a key the account does not allow it for", async () => {
    renderTable()
    expect(
      await screen.findByRole("button", { name: "Revoke deploy" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Revoke seed" })
    ).not.toBeInTheDocument()
  })

  it("shows a placeholder line below the rows while more keys load", async () => {
    state.loadingMore = true
    const { container } = renderTable()
    await screen.findByText("sok_k1…")
    expect(container.querySelectorAll('[aria-busy="true"]')).toHaveLength(1)
    state.loadingMore = false
  })
})
