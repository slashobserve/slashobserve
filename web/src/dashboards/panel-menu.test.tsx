import { createRef } from "react"
import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { LayoutPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { PanelMenu } from "./panel-menu"

const panel: LayoutPanel = {
  id: "p1",
  type: "timeseries",
  title: "Checkout latency",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ query: "up" }],
}

let finishExit = () => {}

function holdMenuExit() {
  const exiting = new Promise<void>((resolve) => {
    finishExit = resolve
  })
  const getAnimations = Element.prototype.getAnimations
  vi.spyOn(Element.prototype, "getAnimations").mockImplementation(function (
    this: Element
  ) {
    if (this.getAttribute("data-slot") !== "dropdown-menu-content") {
      return getAnimations ? getAnimations.call(this) : []
    }
    return [{ finished: exiting }] as unknown as Animation[]
  })
}

function renderMenu() {
  const composer = createRef<HTMLTextAreaElement>()
  const view = renderWithProviders(
    <>
      <PanelMenu
        panel={panel}
        tableView={false}
        onToggleTableView={() => {}}
        idle
        viewing={false}
        canComment
        canEdit={false}
        composerRef={composer}
      />
      <textarea ref={composer} aria-label="Comment or request" />
      <button type="button">Elsewhere</button>
    </>,
    { route: { path: "/d/$dashboardId/{-$slug}", url: "/d/d1" } }
  )
  return view
}

async function chooseComment(user: ReturnType<typeof renderMenu>["user"]) {
  await user.click(
    await screen.findByRole("button", { name: "Panel menu: Checkout latency" })
  )
  await user.click(await screen.findByRole("menuitem", { name: "Comment" }))
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ available: false }), {
          headers: { "Content-Type": "application/json" },
        })
    )
  )
  holdMenuExit()
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe("PanelMenu", () => {
  it("puts the cursor in the comment box once the menu has closed after Comment", async () => {
    const { user } = renderMenu()
    await chooseComment(user)
    finishExit()
    await waitFor(() =>
      expect(
        screen.getByRole("textbox", { name: "Comment or request" })
      ).toHaveFocus()
    )
  })

  it("leaves focus where the person clicked when the menu finishes closing after they moved on", async () => {
    const { user } = renderMenu()
    await chooseComment(user)
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" })
    await user.click(elsewhere)
    finishExit()
    await waitFor(() =>
      expect(screen.queryByRole("menu")).not.toBeInTheDocument()
    )
    expect(elsewhere).toHaveFocus()
  })
})
