import { screen } from "@testing-library/react"
import { beforeAll, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { FindingCard } from "./finding-card"
import type { Finding } from "./queries"

vi.mock("./evidence-panel", () => ({
  EvidencePanel: ({ panel }: { panel: { title: string } }) => (
    <article aria-label={panel.title} />
  ),
}))

const permissions = vi.hoisted(() => ({ edit: false }))

vi.mock("@/dashboards/permissions", () => ({
  useLatestPermissions: () => ({
    comment: true,
    edit: permissions.edit,
    manage: false,
    create: false,
  }),
}))

const text = "The error rate **doubled** at 10:02."

function finding(): Finding {
  return {
    id: "f1",
    observation_id: "f1",
    agent_request_id: "r1",
    dashboard_id: null,
    folder_id: null,
    ask_on: null,
    title: "Checkout errors rose after the deploy",
    markdown: text,
    evidence: [
      { id: "evidence-1", type: "timeseries", title: "Error rate" },
    ] as unknown as Finding["evidence"],
    from: "2026-11-06T10:00:00Z",
    to: "2026-11-06T11:00:00Z",
    annotation_id: null,
    created_by: { kind: "user", id: "u1", name: "Ada" },
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: "r1",
      label: "Observe Agent for Ada",
    },
    created_at: "2026-11-06T11:05:00Z",
    url: "",
    suggested_change: null,
    observe_worthy: false,
    restricted: false,
  }
}

function renderCard(variant?: "card" | "page", showOpenLink = false) {
  return renderWithProviders(
    <FindingCard
      finding={{ ...finding(), observation_id: "o9" }}
      showOpenLink={showOpenLink}
      variant={variant}
    />,
    { route: { path: "/", url: "/" } }
  )
}

function renderUnsaved(saveAnswerOf?: string) {
  return renderWithProviders(
    <FindingCard
      finding={{ ...finding(), observation_id: null, url: null }}
      showOpenLink
      saveAnswerOf={saveAnswerOf}
    />,
    { route: { path: "/", url: "/" } }
  )
}

async function frameOf() {
  const range = await screen.findByText(/^(Found|Evidence from)/)
  return { range, frame: range.parentElement }
}

describe("FindingCard", () => {
  beforeAll(() => import("./finding-markdown"))

  it("draws the bordered card by default", async () => {
    renderCard()
    const { frame, range } = await frameOf()
    expect(frame).toHaveClass("rounded-card", "border")
    expect(range).toHaveTextContent(/^Found/)
    expect(await screen.findByText("doubled")).toBeInTheDocument()
    expect(screen.getByRole("article", { name: "Error rate" })).toBeVisible()
  })

  it("reads as a page with a 15px measured text and no border", async () => {
    renderCard("page")
    const { frame } = await frameOf()
    expect(frame).not.toHaveClass("border")
    expect(frame).not.toHaveClass("rounded-card")
    const body = (await screen.findByText("doubled")).closest(
      '[class~="text-[15px]"]'
    )
    expect(body).toHaveClass("max-w-[680px]")
    expect(body?.querySelector('[class*="text-"]')).toBeNull()
    expect(screen.getByRole("article", { name: "Error rate" })).toBeVisible()
  })

  it("links to the finding's observation, not to the finding", async () => {
    renderCard("card", true)
    const link = await screen.findByRole("link", { name: "Open observation" })
    expect(link).toHaveAttribute("href", "/o/o9")
  })

  it("names the evidence range on the page", async () => {
    renderCard("page")
    const { range } = await frameOf()
    expect(range.textContent).toMatch(/^Evidence from .+\.$/)
  })

  it("keeps an unsaved answer to the chat for a reader who did not ask", async () => {
    renderUnsaved()
    await frameOf()
    expect(screen.queryByRole("link", { name: "Open observation" })).toBeNull()
    expect(
      screen.queryByRole("button", { name: "Save as observation" })
    ).toBeNull()
    expect(screen.queryByRole("button", { name: "Pin" })).toBeNull()
  })

  it("offers its requester to save the answer as an observation and then links to it", async () => {
    const requested: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        requested.push(`${request.method} ${new URL(request.url).pathname}`)
        return new Response(
          JSON.stringify({
            observation_id: "o5",
            url: "/o/o5",
            replayed: false,
          }),
          { status: 201, headers: { "Content-Type": "application/json" } }
        )
      })
    )
    renderUnsaved("r1")
    const save = await screen.findByRole("button", {
      name: "Save as observation",
    })
    save.click()
    const link = await screen.findByRole("link", { name: "Open observation" })
    expect(link).toHaveAttribute("href", "/o/o5")
    expect(requested).toEqual(["POST /api/v1/observe/r1/observation"])
    vi.unstubAllGlobals()
  })

  it("opens a made dashboard with its chat in the side panel", async () => {
    const dashboardId = "33333333-3333-4333-8333-333333333333"
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              dashboard: { id: dashboardId, title: "Checkout errors" },
            }),
            { status: 201, headers: { "Content-Type": "application/json" } }
          )
      )
    )
    const { user, router } = renderWithProviders(
      <FindingCard
        finding={{ ...finding(), observation_id: null, url: null }}
        showOpenLink
        saveAnswerOf="r1"
        chatId="c1"
      />,
      { route: { path: "/", url: "/" } }
    )
    await user.click(
      await screen.findByRole("button", { name: "Make a dashboard" })
    )
    await vi.waitFor(() =>
      expect(router?.state.location.pathname).toBe(
        `/d/${dashboardId}/checkout-errors`
      )
    )
    expect(router?.state.location.search).toEqual({ tab: "chat", chat: "c1" })
    vi.unstubAllGlobals()
  })

  it("puts extra answer actions in the same row as Copy answer", async () => {
    renderWithProviders(
      <FindingCard
        finding={finding()}
        showOpenLink={false}
        actions={<button type="button">Ask again</button>}
      />,
      { route: { path: "/", url: "/" } }
    )
    const copy = await screen.findByRole("button", { name: "Copy answer" })
    expect(copy.parentElement).toContainElement(
      screen.getByRole("button", { name: "Ask again" })
    )
  })

  it("offers Pin to dashboard only with Edit on the dashboard", async () => {
    const onDashboard = { ...finding(), dashboard_id: "d1" }
    permissions.edit = false
    const view = renderWithProviders(
      <FindingCard finding={onDashboard} showOpenLink={false} />,
      { route: { path: "/", url: "/" } }
    )
    await frameOf()
    expect(
      screen.queryByRole("button", { name: "Pin to dashboard" })
    ).not.toBeInTheDocument()
    view.unmount()
    permissions.edit = true
    renderWithProviders(
      <FindingCard finding={onDashboard} showOpenLink={false} />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("button", { name: "Pin to dashboard" })
    ).toBeVisible()
  })

  it("still shows a pinned finding to someone who cannot pin", async () => {
    permissions.edit = false
    renderWithProviders(
      <FindingCard
        finding={{ ...finding(), dashboard_id: "d1", annotation_id: "a1" }}
        showOpenLink={false}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(await screen.findByText("Pinned to the dashboard")).toBeVisible()
  })
})

describe("FindingCard copy", () => {
  it("copies the answer text", async () => {
    const { user } = renderCard()
    const writeText = vi.spyOn(navigator.clipboard, "writeText")
    await user.click(await screen.findByRole("button", { name: "Copy answer" }))
    expect(writeText).toHaveBeenCalledWith(text)
  })
})
