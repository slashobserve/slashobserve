import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { UnknownSource } from "./unknown-source"

const route = { path: "$", url: "/logs/clickhouse" }

describe("UnknownSource", () => {
  it.each([
    ["logs", "Logs", "/logs"],
    ["traces", "Traces", "/traces"],
    ["metrics", "Metrics", "/explore/metrics"],
  ] as const)(
    "names the missing %s datasource and links back",
    async (kind, page, path) => {
      renderWithProviders(<UnknownSource kind={kind} uid="clickhouse" />, {
        route,
      })
      expect(
        await screen.findByRole("heading", {
          name: `No ${page.toLowerCase()} datasource named clickhouse`,
        })
      ).toBeInTheDocument()
      expect(
        screen.getByRole("link", { name: `Open ${page}` })
      ).toHaveAttribute("href", path)
    }
  )

  it("shows what it is given beside the link", async () => {
    renderWithProviders(
      <UnknownSource kind="traces" uid="x">
        <p>Open Tempo</p>
      </UnknownSource>,
      { route }
    )
    expect(await screen.findByText("Open Tempo")).toBeInTheDocument()
  })
})
