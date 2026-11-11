import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { FindingPlace } from "./finding-place"
import type { FindingEnvelope } from "./queries"

const folderId = "22222222-2222-4222-8222-222222222222"

function envelope(
  dashboardId: string | null,
  folder: { title: string; personal: boolean } | null
): FindingEnvelope {
  return {
    finding: { dashboard_id: dashboardId },
    folder:
      folder === null ? null : { id: folderId, general: false, ...folder },
  } as unknown as FindingEnvelope
}

function place(value: FindingEnvelope) {
  renderWithProviders(<FindingPlace envelope={value} />, {
    route: { path: "/", url: "/" },
  })
}

describe("FindingPlace", () => {
  it("links the folder a filed finding is in", async () => {
    place(envelope(null, { title: "Shop", personal: false }))
    const link = await screen.findByRole("link", { name: "Shop" })
    expect(link).toHaveAttribute("href", `/f/${folderId}`)
    expect(link.parentElement).toHaveTextContent("In Shop")
  })

  it("says My Dashboards for a personal folder", async () => {
    place(envelope(null, { title: "Ana", personal: true }))
    expect(
      await screen.findByRole("link", { name: "My Dashboards" })
    ).toBeInTheDocument()
  })

  it("names the dashboard's folder for a finding on a dashboard", async () => {
    place(envelope("d1", { title: "Shop", personal: false }))
    const link = await screen.findByRole("link", { name: "Shop" })
    expect(link.parentElement).toHaveTextContent(
      "On its dashboard's folder, Shop"
    )
  })

  it("says no folder for an unfiled finding", async () => {
    place(envelope(null, null))
    expect(await screen.findByText("In no folder")).toBeInTheDocument()
  })
})
