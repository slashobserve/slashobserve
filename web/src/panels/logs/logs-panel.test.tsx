import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import type { LogEntry, LogsDisplay } from "./logs-model"
import { columnTemplate, LogsView, type LogsViewProps } from "./logs-panel"

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

describe("row tab stop", () => {
  it("makes one row the tab stop and follows the focused row", async () => {
    const user = userEvent.setup()
    view({})
    const buttons = screen.getAllByRole("button")
    expect(buttons.map((button) => button.getAttribute("tabindex"))).toEqual([
      "0",
      ...buttons.slice(1).map(() => "-1"),
    ])
    buttons[1].focus()
    await user.keyboard("j")
    const stops = screen
      .getAllByRole("button")
      .filter((button) => button.getAttribute("tabindex") === "0")
    expect(stops).toEqual([buttons[2]])
  })
})

describe("columnTemplate", () => {
  it.each([
    [
      { showTime: true, levelColumn: true, count: 1 },
      "176px 92px 100px minmax(0,1fr)",
    ],
    [
      { showTime: true, levelColumn: false, count: 2 },
      "176px 100px 100px minmax(0,1fr)",
    ],
    [{ showTime: false, levelColumn: true, count: 0 }, "92px minmax(0,1fr)"],
    [{ showTime: false, levelColumn: false, count: 0 }, "minmax(0,1fr)"],
  ])("lays out %j as the board does", (layout, template) => {
    expect(columnTemplate(layout)).toBe(template)
  })
})

describe("log lines in a narrow list", () => {
  it("stacks the message under the time and level in the default layout", () => {
    view()
    const list = screen.getByRole("list", { name: "Live tail log lines" })
    expect(list).toHaveClass("@container")
    const message = list.querySelector("[data-logs-message]")
    expect(message?.parentElement).toHaveClass("@max-xl:basis-full")
    expect(message?.closest("button")).toHaveClass("@max-xl:flex-wrap")
  })

  it("leaves chosen columns on their grid", () => {
    view({ columns: ["host"] })
    const message = screen
      .getByRole("list", { name: "Live tail log lines" })
      .querySelector("[data-logs-message]")
    expect(message?.parentElement).not.toHaveClass("@max-xl:basis-full")
  })
})
