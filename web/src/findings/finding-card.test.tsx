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

const text = "The error rate **doubled** at 10:02."

function finding(): Finding {
  return {
    id: "f1",
    observation_id: "f1",
    agent_request_id: "r1",
    dashboard_id: null,
    folder_id: null,
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
      label: "/observe for Ada",
    },
    created_at: "2026-11-06T11:05:00Z",
    url: "",
  }
}

function renderCard(variant?: "card" | "page") {
  return renderWithProviders(
    <FindingCard finding={finding()} showOpenLink={false} variant={variant} />,
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

  it("names the evidence range on the page", async () => {
    renderCard("page")
    const { range } = await frameOf()
    expect(range.textContent).toMatch(/^Evidence from .+\.$/)
  })
})
