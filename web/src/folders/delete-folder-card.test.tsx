import { screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { DeleteFolderCard } from "./delete-folder-card"
import type { Folder } from "./queries"

const folder = {
  id: "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77",
  title: "Shop",
  dashboard_count: 0,
} as Folder

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("DeleteFolderCard", () => {
  it("says a refused delete means findings or saved searches are still in the folder", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json(
          {
            error: "folder still holds content",
            code: "folder_not_empty",
          },
          409
        )
      )
    )
    const { user } = renderWithProviders(<DeleteFolderCard folder={folder} />, {
      route: { path: "/", url: "/" },
    })

    await user.click(
      await screen.findByRole("button", { name: "Delete folder" })
    )
    const dialog = await screen.findByRole("alertdialog")
    await user.click(
      within(dialog).getByRole("button", { name: "Delete folder" })
    )

    expect(
      await within(dialog).findByText(
        "This folder still holds dashboards, findings or saved searches. Move or delete them first."
      )
    ).toBeInTheDocument()
  })
})
