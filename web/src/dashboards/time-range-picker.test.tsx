import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { quickRanges } from "@/lib/time"
import { renderWithProviders } from "@/test/render"

import { TimeRangeControl, TimeRangePicker } from "./time-range-picker"

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

describe("TimeRangeControl", () => {
  function renderControl() {
    const onChange = vi.fn()
    const result = renderWithProviders(
      <TimeRangeControl
        value={{ from: "now-1h", to: "now" }}
        onChange={onChange}
      />
    )
    return { ...result, onChange }
  }

  async function open(user: ReturnType<typeof renderControl>["user"]) {
    await user.click(
      await screen.findByRole("button", { name: "Time range: Last 1 hour" })
    )
  }

  it("reports a quick range", async () => {
    const { user, onChange } = renderControl()
    await open(user)
    await user.click(screen.getByRole("button", { name: "Last 6 hours" }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: "now-6h",
      to: "now",
    })
  })

  it("reports a typed range", async () => {
    const { user, onChange } = renderControl()
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, "now-2d")
    await user.click(screen.getByRole("button", { name: "Apply" }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: "now-2d",
      to: "now",
    })
  })

  it("shows the alert and reports nothing for an unreadable time", async () => {
    const { user, onChange } = renderControl()
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, "yesterday")
    await user.click(screen.getByRole("button", { name: "Apply" }))
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Enter a time like now-6h, now/d or 2026-10-17 09:00."
    )
    expect(onChange).not.toHaveBeenCalled()
  })
})
