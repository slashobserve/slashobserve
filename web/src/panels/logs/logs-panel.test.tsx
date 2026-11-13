import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import type { LogEntry, LogsDisplay } from "./logs-model"
import { LogsView, type LogsViewProps } from "./logs-panel"

const panel = {
  id: "p1",
  title: "Panel",
  gridPos: { x: 0, y: 0, w: 12, h: 8 },
  type: "logs",
  datasource: "ch",
  queries: [{ query: "SELECT 1", format: "logs" }],
} as DataPanel

const times = [1_000, 2_000, 3_000, 4_000]

const frames: PreparedFrame[] = [
  {
    ref: "A",
    fields: [
      { name: "Time", type: "time", values: times },
      {
        name: "Body",
        type: "string",
        values: ["first", "second", "third", "fourth"],
      },
    ],
  },
]

function view(extra: Partial<LogsViewProps> = {}) {
  return render(
    <LogsView
      panelTitle="Live tail"
      frames={frames}
      fields={createFieldResolver(panel)}
      zone="UTC"
      display={
        displayWithDefaults("logs", {
          order: "oldest",
          details: true,
          detailsMode: "sidebar",
        }) as LogsDisplay
      }
      width={800}
      height={400}
      {...extra}
    />
  )
}

function atTime(...wanted: number[]) {
  return (entry: LogEntry) => entry.time !== null && wanted.includes(entry.time)
}

describe("new lines in the log view", () => {
  it("adds ', new' to the rows that arrived and to no other", () => {
    view({ isNew: atTime(3_000, 4_000) })
    const rows = screen.getAllByRole("listitem")
    expect(rows).toHaveLength(4)
    expect(rows.map((row) => row.hasAttribute("data-new"))).toEqual([
      false,
      false,
      true,
      true,
    ])
    expect(within(rows[2]).getByText(", new")).toBeInTheDocument()
    expect(within(rows[0]).queryByText(", new")).not.toBeInTheDocument()
  })

  it("does not call an arrived row selected", () => {
    view({ isNew: atTime(4_000) })
    expect(screen.queryByText("Selected line")).not.toBeInTheDocument()
  })

  it("draws the divider above the first row it names and nowhere else", () => {
    view({ newSince: { isFirst: atTime(3_000), label: "New since 14:14" } })
    const dividers = screen.getAllByRole("separator", {
      name: "New since 14:14",
    })
    expect(dividers).toHaveLength(1)
    const rows = screen.getAllByRole("listitem")
    expect(within(rows[2]).getByRole("separator")).toBe(dividers[0])
    expect(within(rows[1]).queryByRole("separator")).not.toBeInTheDocument()
  })

  it("moves between rows with j and k across the divider", async () => {
    const user = userEvent.setup()
    view({ newSince: { isFirst: atTime(3_000), label: "New since 14:14" } })
    const buttons = screen.getAllByRole("button")
    buttons[1].focus()
    await user.keyboard("j")
    expect(buttons[2]).toHaveFocus()
    await user.keyboard("k")
    expect(buttons[1]).toHaveFocus()
  })
})
