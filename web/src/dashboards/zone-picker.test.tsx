import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ZonePicker } from "./zone-picker"

function show(value: string, dashboardZone: string | undefined) {
  return renderWithProviders(
    <ZonePicker value={value} dashboardZone={dashboardZone} />,
    { route: { path: "/dashboards/$dashboardId", url: "/dashboards/d1" } }
  )
}

describe("ZonePicker", () => {
  it("shows the zone the dashboard is in", async () => {
    show("utc", "utc")
    expect(
      await screen.findByRole("combobox", { name: "Time zone: UTC" })
    ).toBeInTheDocument()
  })

  it("puts a chosen zone in the address", async () => {
    const { user, router } = show("browser", undefined)
    await user.click(
      await screen.findByRole("combobox", { name: "Time zone: Browser time" })
    )
    await user.click(await screen.findByRole("option", { name: "UTC" }))
    expect(router?.state.location.search).toMatchObject({ tz: "utc" })
  })

  it("drops the address parameter when the dashboard's own zone is chosen again", async () => {
    const { user, router } = show("utc", undefined)
    await user.click(
      await screen.findByRole("combobox", { name: "Time zone: UTC" })
    )
    await user.click(
      await screen.findByRole("option", { name: "Browser time" })
    )
    expect(router?.state.location.search).not.toHaveProperty("tz")
  })
})
