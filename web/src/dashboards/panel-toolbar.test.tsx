import { createRef } from "react"
import { screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { LayoutPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { PanelToolbar } from "./panel-toolbar"

const panel: LayoutPanel = {
  id: "p1",
  type: "timeseries",
  title: "Checkout latency",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ query: "up" }],
}

const viewer = { comment: false, edit: false }
const commenter = { comment: true, edit: false }
const editor = { comment: true, edit: true }

function serveAgent(available: boolean) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ available }), {
          headers: { "Content-Type": "application/json" },
        })
    )
  )
}

function renderToolbar(
  permissions: { comment: boolean; edit: boolean },
  over: { idle?: boolean; panel?: LayoutPanel } = {}
) {
  return renderWithProviders(
    <PanelToolbar
      panel={over.panel ?? panel}
      permissions={permissions}
      composerRef={createRef<HTMLTextAreaElement>()}
      from={undefined}
      to={undefined}
      idle={over.idle ?? true}
      menu={
        <button type="button" aria-label="More actions for Checkout latency" />
      }
    />,
    { route: { path: "/", url: "/" } }
  )
}

function buttonNames(): string[] {
  return screen
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? "")
}

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
})

afterEach(() => vi.unstubAllGlobals())

describe("PanelToolbar", () => {
  it("is a labelled toolbar for the panel", async () => {
    serveAgent(true)
    renderToolbar(editor)
    expect(
      await screen.findByRole("toolbar", {
        name: "Actions for Checkout latency",
      })
    ).toBeVisible()
  })

  it("shows a viewer only Add to observation and More", async () => {
    serveAgent(true)
    renderToolbar(viewer)
    await screen.findByRole("button", { name: "Add to observation" })
    expect(buttonNames()).toEqual([
      "Add to observation",
      "More actions for Checkout latency",
    ])
  })

  it("lets a commenter comment and suggest an edit but not ask the agent", async () => {
    serveAgent(true)
    renderToolbar(commenter)
    await screen.findByRole("button", { name: "Comment" })
    expect(buttonNames()).toEqual([
      "Comment",
      "Suggest an edit",
      "Add to observation",
      "More actions for Checkout latency",
    ])
  })

  it("shows an editor every action once the agent is available", async () => {
    serveAgent(true)
    renderToolbar(editor)
    await screen.findByRole("button", { name: "Ask @observe" })
    expect(buttonNames()).toEqual([
      "Comment",
      "Ask @observe",
      "Edit query",
      "Add to observation",
      "More actions for Checkout latency",
    ])
  })

  it("leaves out Ask while the agent is unavailable", async () => {
    serveAgent(false)
    renderToolbar(editor)
    await screen.findByRole("button", { name: "Comment" })
    expect(
      screen.queryByRole("button", { name: "Ask @observe" })
    ).not.toBeInTheDocument()
  })

  it("greys Add to observation for a panel that uses variables", async () => {
    serveAgent(true)
    renderToolbar(viewer, {
      panel: { ...panel, queries: [{ query: 'up{job=~"$job"}' }] },
    })
    expect(
      await screen.findByRole("button", { name: "Add to observation" })
    ).toBeDisabled()
  })

  it("disables the actions that need a finished layout while it has unsaved moves", async () => {
    serveAgent(true)
    renderToolbar(editor, { idle: false })
    await screen.findByRole("button", { name: "Ask @observe" })
    for (const name of [
      "Comment",
      "Ask @observe",
      "Edit query",
      "Add to observation",
    ]) {
      expect(screen.getByRole("button", { name })).toBeDisabled()
    }
  })

  it("moves focus between buttons with the arrow keys, Home and End", async () => {
    serveAgent(true)
    const { user } = renderToolbar(editor)
    await screen.findByRole("button", { name: "Ask @observe" })
    const comment = screen.getByRole("button", { name: "Comment" })
    comment.focus()
    expect(comment).toHaveAttribute("tabindex", "0")
    expect(
      screen.getByRole("button", { name: "Ask @observe" })
    ).toHaveAttribute("tabindex", "-1")

    await user.keyboard("{ArrowRight}")
    expect(screen.getByRole("button", { name: "Ask @observe" })).toHaveFocus()
    await user.keyboard("{End}")
    expect(
      screen.getByRole("button", { name: "More actions for Checkout latency" })
    ).toHaveFocus()
    await user.keyboard("{ArrowRight}")
    expect(
      screen.getByRole("button", { name: "More actions for Checkout latency" })
    ).toHaveFocus()
    await user.keyboard("{ArrowLeft}")
    expect(
      screen.getByRole("button", { name: "Add to observation" })
    ).toHaveFocus()
    await user.keyboard("{Home}")
    expect(comment).toHaveFocus()
    expect(comment).toHaveAttribute("tabindex", "0")
  })
})
