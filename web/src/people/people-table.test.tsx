import { screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders, testQueryClient } from "@/test/render"

import { PeopleTable, peopleColumns } from "./people-table"
import { usersQuery, type User } from "./queries"

vi.mock("./person-menu", () => ({
  PersonMenu: ({ user }: { user: User }) => (
    <button type="button">Actions for {user.name}</button>
  ),
}))
vi.mock("./role-select", () => ({
  PersonRole: ({ person }: { person: User }) => <span>{person.role}</span>,
}))
vi.mock("./sign-out-everywhere-button", () => ({
  SignOutEverywhereButton: ({ user }: { user: User }) => (
    <button type="button">Sign out {user.name}</button>
  ),
}))

const active = {
  id: "u1",
  name: "Ada Lovelace",
  email: "ada@example.com",
  role: "editor",
  dashboard_count: 3,
  key_count: 2,
  last_seen_at: null,
  deactivated_at: null,
  deactivated_by: null,
} as unknown as User

const deactivated = {
  ...active,
  id: "u2",
  name: "Grace Hopper",
  email: "grace@example.com",
  deactivated_at: "2026-11-01T00:00:00Z",
  deactivated_by: "Ada Lovelace",
} as User

function renderTable(people: User[]) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(usersQuery().queryKey, people)
  return renderWithProviders(<PeopleTable />, { queryClient })
}

describe("PeopleTable", () => {
  it("draws a row per person, tagged with the person's id", async () => {
    const { container } = renderTable([active, deactivated])
    await screen.findByRole("gridcell", { name: "ada@example.com" })

    expect(
      screen.getAllByRole("columnheader").map((header) => header.textContent)
    ).toEqual([
      "Name",
      "Email",
      "Role",
      "Dashboards",
      "API keys",
      "Last active",
      "Actions",
    ])
    const row = container.querySelector<HTMLElement>('[data-user-id="u1"]')
    expect(row).not.toBeNull()
    expect(within(row as HTMLElement).getByText("Never")).toBeInTheDocument()
    expect(
      within(row as HTMLElement).getByRole("button", {
        name: "Sign out Ada Lovelace",
      })
    ).toBeInTheDocument()
  })

  it("fits its columns within the card at 1440 px", () => {
    const total = peopleColumns(0).reduce(
      (sum, column) => sum + (column.width ?? column.minWidth ?? 0),
      0
    )

    expect(total).toBeLessThanOrEqual(1100)
  })

  it("keeps each email on one line and shows it whole on hover", async () => {
    renderTable([active])
    const cell = await screen.findByRole("gridcell", {
      name: "ada@example.com",
    })

    expect(within(cell).getByTitle("ada@example.com")).toHaveClass("truncate")
  })

  it("shows who deactivated a person and offers no sign-out for them", async () => {
    const { container } = renderTable([active, deactivated])
    await screen.findByRole("gridcell", { name: "grace@example.com" })

    const row = container.querySelector<HTMLElement>(
      '[data-user-id="u2"]'
    ) as HTMLElement
    expect(
      within(row).getByText("Deactivated by Ada Lovelace")
    ).toBeInTheDocument()
    expect(
      within(row).queryByRole("button", { name: /Sign out/ })
    ).not.toBeInTheDocument()
    expect(
      within(row).getByRole("button", { name: "Actions for Grace Hopper" })
    ).toBeInTheDocument()
  })

  it("says so when nobody has signed in", () => {
    renderTable([])
    expect(screen.getByText("Nobody has signed in yet.")).toBeInTheDocument()
  })
})
