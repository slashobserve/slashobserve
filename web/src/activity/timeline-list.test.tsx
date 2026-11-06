import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { TimelineList } from "./timeline-list"
import type { TimelineEntry } from "./timeline-model"

function running(key: string, at: string): TimelineEntry {
  return {
    kind: "running",
    key,
    at,
    panelId: null,
    request: {
      id: key,
      status: "running",
      dashboard_id: "dashboard",
      panel_id: null,
      created_at: at,
      started_at: at,
    },
  }
}

const older = { available: false, fetching: false, error: null, onLoad() {} }

function list(entries: TimelineEntry[], filtered = false) {
  return render(
    <TimelineList
      entries={entries}
      loading={false}
      error={false}
      onRetry={() => {}}
      filtered={filtered}
      older={older}
      renderEntry={(entry) => <li>{entry.key}</li>}
    />
  )
}

describe("TimelineList", () => {
  it("labels each day before its first entry", () => {
    const today = new Date()
    const yesterday = new Date(
      today.getFullYear(),
      today.getMonth(),
      today.getDate() - 1,
      12
    )
    list([
      running("first", yesterday.toISOString()),
      running("second", today.toISOString()),
    ])
    const rows = within(screen.getByRole("list", { name: "Activity" }))
      .getAllByRole("listitem")
      .map((row) => row.textContent)
    expect(rows).toEqual(["Yesterday", "first", "Today", "second"])
  })

  it("explains an empty timeline", () => {
    list([])
    expect(screen.getByText("No activity yet.")).toBeInTheDocument()
    expect(
      screen.getByText("Comment on a panel from its menu, or write below.")
    ).toBeInTheDocument()
  })

  it("says when the filters hide everything", () => {
    list([], true)
    expect(
      screen.getByText("No activity matches these filters.")
    ).toBeInTheDocument()
    expect(screen.queryByText("No activity yet.")).toBeNull()
  })
})
