import { createRef } from "react"
import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { LayoutPanel } from "@/spec"
import { DatasourceListProvider } from "@/datasources/queries"
import { renderWithProviders } from "@/test/render"

import { useCanvasTool } from "./canvas-tools"
import { PanelMenu } from "./panel-menu"

vi.mock("@/observations/add-to-observation-panel", () => ({
  AddToObservationPanel: ({ from, to }: { from?: string; to?: string }) => (
    <div data-testid="add-to-observation">{`${from ?? ""}|${to ?? ""}`}</div>
  ),
}))

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

const logsPanel: LayoutPanel = {
  id: "p2",
  type: "logs",
  title: "Checkout logs",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "loki",
  queries: [{ query: '{app="checkout"}' }],
}

const variableLogsPanel: LayoutPanel = {
  ...logsPanel,
  id: "p3",
  title: "Variable logs",
  datasource: "${logs}",
}

const dashboardRange = { from: "now-24h", to: "now" }

function renderMenu(
  canComment = true,
  downloadsReady?: boolean,
  shown: LayoutPanel = panel,
  onCanvas = false,
  canPropose = false
) {
  const composer = createRef<HTMLTextAreaElement>()
  const view = renderWithProviders(
    <>
      <PanelMenu
        panel={shown}
        range={dashboardRange}
        selections={{ logs: ["loki"] }}
        tableView={false}
        onToggleTableView={() => {}}
        idle
        viewing={false}
        onCanvas={onCanvas}
        canComment={canComment}
        canEdit={false}
        canPropose={canPropose}
        composerRef={composer}
        onDownloadCsv={() => {}}
        onSavePng={() => {}}
        downloadsReady={downloadsReady}
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

async function openMenu(
  user: ReturnType<typeof renderMenu>["user"],
  title: string
) {
  await user.click(
    await screen.findByRole("button", { name: `Panel menu: ${title}` })
  )
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe("PanelMenu", () => {
  it.each([
    [true, true],
    [false, false],
  ])(
    "with canPropose %s, Suggest an edit is shown: %s",
    async (canPropose, shown) => {
      const { user } = renderMenu(true, undefined, panel, false, canPropose)
      await user.click(
        await screen.findByRole("button", {
          name: "Panel menu: Checkout latency",
        })
      )
      await screen.findByRole("menuitem", { name: "Comment" })
      expect(
        screen.queryByRole("menuitem", { name: "Suggest an edit" }) !== null
      ).toBe(shown)
    }
  )

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

  it("opens the canvas composer on the whole panel and leaves the comment box alone where the panel takes canvas comments", async () => {
    const { user } = renderMenu(true, undefined, panel, true)
    await chooseComment(user)
    finishExit()
    await waitFor(() =>
      expect(useCanvasTool.getState().draft).toMatchObject({
        ownerId: panel.id,
        tool: "comment",
        anchor: null,
      })
    )
    expect(
      screen.getByRole("textbox", { name: "Comment or request" })
    ).not.toHaveFocus()
    useCanvasTool.getState().close()
  })

  it("opens the canvas composer on the whole block of an observation on the canvas", async () => {
    const { user } = renderWithProviders(
      <PanelMenu
        panel={panel}
        range={dashboardRange}
        selections={{}}
        tableView={false}
        onToggleTableView={() => {}}
        idle
        viewing={false}
        onCanvas
        ownerId="b1"
        canComment
        canEdit={false}
        canPropose={false}
        composerRef={createRef<HTMLTextAreaElement>()}
        block={{ actions: [] }}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("button", { name: "Comment on Checkout latency" })
    ).toBeInTheDocument()
    await chooseComment(user)
    expect(useCanvasTool.getState().draft).toMatchObject({
      ownerId: "b1",
      tool: "comment",
      anchor: null,
    })
    useCanvasTool.getState().close()
  })

  it("offers no Comment on an observation block off the canvas", async () => {
    const { user } = renderWithProviders(
      <PanelMenu
        panel={panel}
        range={dashboardRange}
        selections={{}}
        tableView={false}
        onToggleTableView={() => {}}
        idle
        viewing={false}
        onCanvas={false}
        canComment
        canEdit={false}
        canPropose={false}
        composerRef={createRef<HTMLTextAreaElement>()}
        block={{ actions: [] }}
      />,
      { route: { path: "/", url: "/" } }
    )
    await openMenu(user, "Checkout latency")
    expect(
      screen.queryByRole("menuitem", { name: /Comment/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Comment on Checkout latency" })
    ).not.toBeInTheDocument()
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

  it("lists the downloads greyed until the panel has its data, so the menu does not grow later", async () => {
    const { user } = renderMenu(true, false)
    await user.click(
      await screen.findByRole("button", {
        name: "Panel menu: Checkout latency",
      })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Download CSV" })
    ).toHaveAttribute("aria-disabled", "true")
    expect(
      screen.getByRole("menuitem", { name: "Save as PNG" })
    ).toHaveAttribute("aria-disabled", "true")
  })

  it("enables the downloads once the panel is ready", async () => {
    const { user } = renderMenu(true, true)
    await user.click(
      await screen.findByRole("button", {
        name: "Panel menu: Checkout latency",
      })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Download CSV" })
    ).not.toHaveAttribute("aria-disabled", "true")
  })

  it("offers Ask to a Commenter once the agent is available", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ available: true }), {
            headers: { "Content-Type": "application/json" },
          })
      )
    )
    const { user } = renderMenu()
    await user.click(
      await screen.findByRole("button", {
        name: "Panel menu: Checkout latency",
      })
    )
    expect(await screen.findByRole("menuitem", { name: "Ask" })).toBeVisible()
  })

  it("shows Comment greyed with its reason when the person cannot comment", async () => {
    const { user } = renderMenu(false)
    await user.click(
      await screen.findByRole("button", {
        name: "Panel menu: Checkout latency",
      })
    )
    expect(
      await screen.findByRole("menuitem", {
        name: "Comment (needs Comment access on this folder)",
      })
    ).toHaveAttribute("aria-disabled", "true")
  })

  it("opens Logs over the dashboard's own range when the address carries none", async () => {
    const { user, router } = renderMenu(true, undefined, logsPanel)
    await openMenu(user, "Checkout logs")
    await user.click(
      await screen.findByRole("menuitem", { name: "Open in Logs" })
    )
    await waitFor(() =>
      expect(router?.state.location.pathname).toBe("/logs/loki")
    )
    expect(router?.state.location.search).toMatchObject(dashboardRange)
  })

  it("opens Metrics with the panel's query over the dashboard's range", async () => {
    const composer = createRef<HTMLTextAreaElement>()
    const { user, router } = renderWithProviders(
      <DatasourceListProvider value={[{ uid: "prom", type: "prometheus" }]}>
        <PanelMenu
          panel={panel}
          range={dashboardRange}
          selections={{}}
          tableView={false}
          onToggleTableView={() => {}}
          idle
          viewing={false}
          onCanvas={false}
          canComment
          canEdit={false}
          canPropose={false}
          composerRef={composer}
        />
      </DatasourceListProvider>,
      { route: { path: "/d/$dashboardId/{-$slug}", url: "/d/d1" } }
    )
    await openMenu(user, "Checkout latency")
    await user.click(
      await screen.findByRole("menuitem", { name: "Open in Metrics" })
    )
    await waitFor(() =>
      expect(router?.state.location.pathname).toBe("/explore/metrics/prom")
    )
    expect(router?.state.location.search).toMatchObject({
      q: "up",
      ...dashboardRange,
    })
  })

  it("greys out Open in Explore with its reason when the datasource has no explorer", async () => {
    const { user } = renderMenu()
    await openMenu(user, "Checkout latency")
    expect(
      await screen.findByRole("menuitem", {
        name: "Open in Explore (no explorer for this datasource)",
      })
    ).toHaveAttribute("aria-disabled", "true")
  })

  it("hands Add to observation the dashboard's own range when the address carries none", async () => {
    const { user } = renderMenu()
    await openMenu(user, "Checkout latency")
    await user.click(
      await screen.findByRole("menuitem", { name: "Add to observation" })
    )
    expect(await screen.findByTestId("add-to-observation")).toHaveTextContent(
      "now-24h|now"
    )
  })

  it("resolves a datasource variable from the dashboard's selections when the address carries none", async () => {
    const { user, router } = renderMenu(true, undefined, variableLogsPanel)
    await openMenu(user, "Variable logs")
    await user.click(
      await screen.findByRole("menuitem", { name: "Open in Logs" })
    )
    await waitFor(() =>
      expect(router?.state.location.pathname).toBe("/logs/loki")
    )
  })
})
