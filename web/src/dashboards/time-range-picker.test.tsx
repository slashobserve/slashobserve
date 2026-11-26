import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { usePreferences } from "@/lib/preferences"
import { quickRanges } from "@/lib/time"
import type { Dashboard } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { TimeRangeControl, TimeRangePicker } from "./time-range-picker"

const unreadableTime =
  /^Enter a time like now-6h, now\/d or \d{4}-\d\d-\d\d \d\d:\d\d\.$/

async function applyRange(from: string, to: string) {
  const result = renderWithProviders(
    <TimeRangePicker
      dashboardId="d1"
      version={1}
      from="now-1h"
      to="now"
      quickRanges={quickRanges}
      hidden={false}
      timezone={undefined}
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
    ["yesterday", "now", unreadableTime],
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

describe("TimeRangePicker default range", () => {
  const spec = {
    title: "Shop",
    time: { from: "now-6h", to: "now" },
    panels: [],
  } as unknown as Dashboard

  async function openPicker(
    defaultRange: Parameters<typeof TimeRangePicker>[0]["defaultRange"]
  ) {
    const result = renderWithProviders(
      <TimeRangePicker
        dashboardId="d1"
        version={1}
        from="now-1h"
        to="now"
        quickRanges={quickRanges}
        hidden={false}
        timezone={undefined}
        defaultRange={defaultRange}
      />,
      { route: { path: "/dashboards/$dashboardId", url: "/dashboards/d1" } }
    )
    await result.user.click(
      await screen.findByRole("button", { name: "Time range: Last 1 hour" })
    )
    return result
  }

  it("offers Save as default to someone who can edit the latest version", async () => {
    await openPicker({ spec, version: 3 })
    expect(
      await screen.findByRole("button", { name: "Save as default" })
    ).toBeEnabled()
  })

  it("offers nothing when the dashboard cannot be saved from here", async () => {
    await openPicker(null)
    await screen.findByLabelText("From")
    expect(
      screen.queryByRole("button", { name: "Save as default" })
    ).not.toBeInTheDocument()
  })
})

describe("a range that is too long", () => {
  it("says why next to the From field", async () => {
    await applyRange("1700000000", "now")
    expect(screen.getByLabelText("From")).toHaveAccessibleDescription(
      "The range is longer than 400 days. Choose a From closer to To."
    )
  })
})

describe("TimeRangeControl", () => {
  afterEach(() => usePreferences.setState({ timeZone: "browser" }))

  function renderControl(zone?: string | null) {
    const onChange = vi.fn()
    const result = renderWithProviders(
      <TimeRangeControl
        value={{ from: "now-1h", to: "now" }}
        onChange={onChange}
        zone={zone}
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
    expect(screen.getByRole("alert")).toHaveTextContent(unreadableTime)
    expect(onChange).not.toHaveBeenCalled()
  })

  it("says why a time before the year 2000 is refused", async () => {
    const { user, onChange } = renderControl()
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, "1999-12-31 23:00")
    await user.click(screen.getByRole("button", { name: "Apply" }))
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Times before the year 2000 are not accepted."
    )
    expect(onChange).not.toHaveBeenCalled()
  })

  it("picks days in the calendar and fills From and To", async () => {
    const { user, onChange } = renderControl("UTC")
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, "2026-10-01 00:00")
    await user.click(screen.getByRole("button", { name: "Pick dates" }))
    await user.click(screen.getByRole("button", { name: "17 October 2026" }))
    await user.click(screen.getByRole("button", { name: "20 October 2026" }))
    expect(fromInput).toHaveValue("2026-10-17 00:00")
    expect(screen.getByLabelText("To")).toHaveValue("2026-10-20 23:59:59")
    await user.click(screen.getByRole("button", { name: "Apply" }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: String(Date.UTC(2026, 9, 17)),
      to: String(Date.UTC(2026, 9, 20, 23, 59, 59)),
    })
  })

  it("offers Save as default only when given a saver, and sends the range to it", async () => {
    const onSave = vi.fn()
    const onChange = vi.fn()
    const { user } = renderWithProviders(
      <TimeRangeControl
        value={{ from: "now-1h", to: "now" }}
        onChange={onChange}
        saveDefault={{
          saved: { from: "now-6h", to: "now" },
          pending: false,
          onSave,
        }}
      />
    )
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, "now-2d")
    await user.click(screen.getByRole("button", { name: "Save as default" }))
    expect(onSave).toHaveBeenCalledExactlyOnceWith({
      from: "now-2d",
      to: "now",
    })
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: "now-2d",
      to: "now",
    })
  })

  it("disables Save as default for the range that already is the default", async () => {
    const { user } = renderWithProviders(
      <TimeRangeControl
        value={{ from: "now-1h", to: "now" }}
        onChange={vi.fn()}
        saveDefault={{
          saved: { from: "now-1h", to: "now" },
          pending: false,
          onSave: vi.fn(),
        }}
      />
    )
    await open(user)
    expect(
      await screen.findByRole("button", { name: "Save as default" })
    ).toBeDisabled()
  })

  it("has no Save as default without a saver", async () => {
    const { user } = renderControl()
    await open(user)
    expect(
      screen.queryByRole("button", { name: "Save as default" })
    ).not.toBeInTheDocument()
  })

  it("reads typed text in the dashboard's zone", async () => {
    const onChange = vi.fn()
    const { user } = renderWithProviders(
      <TimeRangeControl
        value={{ from: "now-1h", to: "now" }}
        onChange={onChange}
        zone="Asia/Kolkata"
      />
    )
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, "2026-10-17 09:00")
    await user.click(screen.getByRole("button", { name: "Apply" }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: String(Date.UTC(2026, 9, 17, 3, 30)),
      to: "now",
    })
  })

  async function applyTyped(
    user: ReturnType<typeof renderControl>["user"],
    from: string,
    to: string
  ) {
    await open(user)
    const fromInput = await screen.findByLabelText("From")
    await user.clear(fromInput)
    await user.type(fromInput, from)
    const toInput = screen.getByLabelText("To")
    await user.clear(toInput)
    await user.type(toInput, to)
    await user.click(screen.getByRole("button", { name: "Apply" }))
  }

  it("reads typed times in the zone the person prefers", async () => {
    usePreferences.setState({ timeZone: "Asia/Tokyo" })
    const { user, onChange } = renderControl()
    await applyTyped(user, "2026-10-17 09:00", "2026-10-17 10:00")
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: String(Date.UTC(2026, 9, 17, 0, 0, 0)),
      to: String(Date.UTC(2026, 9, 17, 1, 0, 0)),
    })
  })

  it("reads typed times in the zone it is given before the preference", async () => {
    usePreferences.setState({ timeZone: "Asia/Tokyo" })
    const { user, onChange } = renderControl("UTC")
    await applyTyped(user, "2026-10-17 09:00", "2026-10-17 10:00")
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: String(Date.UTC(2026, 9, 17, 9, 0, 0)),
      to: String(Date.UTC(2026, 9, 17, 10, 0, 0)),
    })
  })

  it("reads browser time when it is told so, whatever the preference", async () => {
    usePreferences.setState({ timeZone: "Asia/Tokyo" })
    const { user, onChange } = renderControl(null)
    await applyTyped(user, "2026-10-17 09:00", "2026-10-17 10:00")
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      from: String(new Date(2026, 9, 17, 9, 0, 0).getTime()),
      to: String(new Date(2026, 9, 17, 10, 0, 0).getTime()),
    })
  })
})
