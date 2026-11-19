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
