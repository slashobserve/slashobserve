import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { folderKeys } from "@/api/keys"
import type { Folder } from "@/folders/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { PublishSection } from "./publish-section"

const folder = {
  id: "f1",
  title: "Payments",
  general: false,
  personal: false,
  restricted: true,
  kind: "restricted",
  allowed_actions: ["create_in", "move_into"],
  dashboard_count: 3,
  grantees: ["Ada", "sre"],
  grantee_count: 2,
  created_by: null,
  permissions: { comment: true, edit: true, manage: false, create: true },
  level: "edit",
} as Folder

afterEach(() => vi.unstubAllGlobals())

function renderSection() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(folderKeys.list(), [folder])
  return renderWithProviders(
    <PublishSection dashboardId="d1" title="Checkout" headingId="h" />,
    { queryClient }
  )
}

describe("PublishSection", () => {
  it("names the audience before it moves anything", async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal("fetch", fetchMock)
    const { user } = renderSection()
    await user.click(await screen.findByRole("button", { name: "Publish" }))
    expect(
      await screen.findByRole("alertdialog", { name: "Publish to Payments?" })
    ).toBeTruthy()
    expect(
      screen.getAllByText(/only the people and groups on its list \(Ada, sre\)/)
        .length
    ).toBeGreaterThan(0)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("moves the dashboard only after the confirmation", async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      )
    )
    vi.stubGlobal("fetch", fetchMock)
    const { user } = renderSection()
    await user.click(await screen.findByRole("button", { name: "Publish" }))
    await user.click(
      await screen.findByRole("button", { name: "Publish to Payments" })
    )
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  })
})
