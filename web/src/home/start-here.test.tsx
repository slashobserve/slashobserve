import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { StartHere } from "./start-here"

describe("StartHere", () => {
  it("links the places a newcomer can read and the docs", async () => {
    renderWithProviders(<StartHere />, { route: { path: "/", url: "/" } })
    const section = await screen.findByRole("region", { name: "Start here" })
    const links = Array.from(section.querySelectorAll("a")).map((link) => [
      link.textContent,
      link.getAttribute("href"),
    ])
    expect(links).toEqual([
      ["DashboardsSee what the team already watches.", "/dashboards"],
      ["LogsSearch logs, one source at a time.", "/logs"],
      ["TracesFollow one request across services.", "/traces"],
      [
        "ObservationsNotes and findings the team keeps while it works a question.",
        "/observations",
      ],
      [
        "DocsWhat each page does and how the agent works.",
        "https://slashobserve.com/docs",
      ],
    ])
  })
})
