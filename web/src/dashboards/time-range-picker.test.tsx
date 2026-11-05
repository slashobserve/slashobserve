import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { quickRanges } from "@/lib/time"
import { renderWithProviders } from "@/test/render"

import { TimeRangePicker } from "./time-range-picker"

async function applyRange(from: string, to: string) {
  const result = renderWithProviders(
    <TimeRangePicker
      dashboardId="d1"
      version={1}
      from="now-1h"
      to="now"
      quickRanges={quickRanges}
      hidden={false}
    />,
    { route: { path: "/dashboards/$dashboardId", url: "/dashboards/d1" } }
  )
  const { user } = result
  await user.click(
    await screen.findByRole("button", { name: "Time range: Last 1 hour" })
  )
  const fromInput = await screen.findByLabelText("From")
  const toInput = screen.getByLabelText("To")
  await user.clear(fromInput)
  await user.type(fromInput, from)
  await user.clear(toInput)
  await user.type(toInput, to)
  await user.click(screen.getByRole("button", { name: "Apply" }))
  return result
}

describe("TimeRangePicker", () => {
  it.each([
    ["now-6h", "now"],
    ["now/d", "now"],
    ["2026-10-17T09:00:00Z", "2026-10-17T10:00:00Z"],
  ])("applies %s to %s to the URL", async (from, to) => {
    const { router } = await applyRange(from, to)
    expect(screen.queryByRole("alert")).not.toBeInTheDocument()
    expect(router?.state.location.search).toMatchObject({ from, to })
  })

  it.each([
    [
      "yesterday",
      "now",
      "Enter a time like now-6h, now/d or 2026-10-17 09:00.",
    ],
    [
      "2026-10-17T10:00:00Z",
      "2026-10-17T09:00:00Z",
      "From must be earlier than To.",
    ],
  ])("rejects %s to %s", async (from, to, message) => {
    const { router } = await applyRange(from, to)
    expect(screen.getByRole("alert")).toHaveTextContent(message)
    expect(router?.state.location.search).not.toHaveProperty("from")
  })
})
