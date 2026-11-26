import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { describeInstant } from "@/lib/time"
import { renderWithProviders } from "@/test/render"

import { BlockList } from "./block-list"
import {
  FindingHeader,
  hasFindingHeader,
  headerBlocks,
  type RunFinding,
  unverifiedLine,
} from "./finding-header"
import type { Block, Observation } from "./queries"

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function finding(overrides: Partial<RunFinding> = {}): RunFinding {
  return {
    run_id: "r1",
    root_cause: null,
    onset: null,
    impact: null,
    confidence: null,
    ruled_out: [],
    next_steps: [],
    no_incident: false,
    unverified: [],
    ...overrides,
  }
}

function block(id: string, position: number, runId: string | null) {
  return {
    id,
    observation_id: "o1",
    kind: "text",
    state: "live",
    position,
    width: "full",
    height: null,
    version: 1,
    body: { markdown: `Text ${id}` },
    snapshot_id: null,
    run_id: runId,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    allowed_actions: [],
  } as Block
}

const full = finding({
  root_cause: {
    component: "checkout",
    kind: "config_or_flag",
    summary: "The retry flag doubled calls.",
  },
  onset: "2026-11-20T10:05:00Z",
  impact: "12% of orders failed.",
  confidence: "high",
  ruled_out: ["the database", "the cache"],
  next_steps: ["Turn the flag off."],
})

describe("the finding header", () => {
  it("shows the cause, onset, impact, confidence and lists", () => {
    render(<FindingHeader finding={full} />)
    const header = screen.getByLabelText("Finding summary")
    expect(header).toHaveTextContent("checkout")
    expect(header).toHaveTextContent("Config or flag")
    expect(header).toHaveTextContent("The retry flag doubled calls.")
    expect(header).toHaveTextContent("12% of orders failed.")
    expect(header).toHaveTextContent("high")
    expect(
      within(header)
        .getAllByRole("listitem")
        .map((item) => item.textContent)
    ).toEqual(["the database", "the cache", "Turn the flag off."])
    expect(header).toHaveTextContent("Turn the flag off.")
    expect(header).toHaveTextContent("Started")
  })

  it("shows the onset with the browser time zone", () => {
    render(<FindingHeader finding={full} />)
    expect(screen.getByLabelText("Finding summary")).toHaveTextContent(
      describeInstant("2026-11-20T10:05:00Z")
    )
  })

  it("says so when nothing was wrong", () => {
    render(<FindingHeader finding={finding({ no_incident: true })} />)
    expect(screen.getByText("No incident found")).toBeInTheDocument()
  })

  it("counts a finding with no field as having no header", () => {
    expect(hasFindingHeader(finding())).toBe(false)
    expect(hasFindingHeader(finding({ confidence: "low" }))).toBe(true)
  })
})

describe("the line for items the agent did not read", () => {
  it("counts figures", () => {
    expect(
      unverifiedLine(
        finding({
          unverified: [
            { kind: "figure", text: "523 ms" },
            { kind: "figure", text: "41%" },
          ],
        })
      )
    ).toBe("2 figures were not found in what the agent read: 523 ms, 41%")
  })

  it("names one figure, the component and the onset", () => {
    expect(
      unverifiedLine(
        finding({ unverified: [{ kind: "figure", text: "3.2K" }] })
      )
    ).toBe("1 figure was not found in what the agent read: 3.2K")
    expect(
      unverifiedLine(
        finding({
          unverified: [
            { kind: "figure", text: "12%" },
            { kind: "component", text: "cartservice" },
            { kind: "onset", text: "2026-11-20T10:05:00Z" },
          ],
        })
      )
    ).toBe(
      "1 figure, the root cause component and the onset time were not found in what the agent read: 12%, cartservice, 2026-11-20T10:05:00Z"
    )
    expect(
      unverifiedLine(
        finding({ unverified: [{ kind: "component", text: "cartservice" }] })
      )
    ).toBe(
      "The root cause component was not found in what the agent read: cartservice"
    )
  })
})

describe("which block carries a header", () => {
  it("picks the first text block of the run and skips a finding with no field", () => {
    const blocks = [
      block("a", 0, null),
      block("c", 2, "r1"),
      block("b", 1, "r1"),
    ]
    const headers = headerBlocks(blocks, [
      full,
      finding({ run_id: "r2", impact: null }),
    ])
    expect([...headers.keys()]).toEqual(["b"])
  })

  it("prefers a live block over an earlier requested one of the same run", () => {
    const requested = { ...block("a", 0, "r1"), state: "requested" } as Block
    const headers = headerBlocks([requested, block("b", 1, "r1")], [full])
    expect([...headers.keys()]).toEqual(["b"])
  })

  it("falls back to the requested block when the run has no live one", () => {
    const requested = { ...block("a", 0, "r1"), state: "requested" } as Block
    expect([...headerBlocks([requested], [full]).keys()]).toEqual(["a"])
  })

  it("makes none when the observation has no findings", () => {
    expect(headerBlocks([block("a", 0, "r1")], undefined).size).toBe(0)
  })
})

describe("the observation's blocks", () => {
  function show(findings: Observation["findings"]) {
    return renderWithProviders(
      <BlockList
        observation={
          { id: "o1", allowed_actions: [], findings } as unknown as Observation
        }
        blocks={[block("a", 0, "r1")]}
        canAdd={false}
        canComment={false}
        reveal={null}
        onRevealed={() => {}}
        onAdded={() => {}}
        footerSlot={null}
      />,
      { route: { path: "/", url: "/" } }
    )
  }

  it("shows the header above the answer", async () => {
    show([full])
    const answer = await screen.findByText("Text a")
    const header = screen.getByLabelText("Finding summary")
    expect(
      header.compareDocumentPosition(answer) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it("shows the unverified line under the header", async () => {
    show([{ ...full, unverified: [{ kind: "figure", text: "523 ms" }] }])
    const line = await screen.findByText(
      "1 figure was not found in what the agent read: 523 ms"
    )
    const header = screen.getByLabelText("Finding summary")
    expect(
      header.compareDocumentPosition(line) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it("shows the unverified line alone when no field is set", async () => {
    show([finding({ unverified: [{ kind: "figure", text: "523 ms" }] })])
    expect(
      await screen.findByText(
        "1 figure was not found in what the agent read: 523 ms"
      )
    ).toBeInTheDocument()
    expect(screen.queryByLabelText("Finding summary")).toBeNull()
  })

  it("shows nothing extra when no field is set", async () => {
    show([finding()])
    expect(await screen.findByText("Text a")).toBeInTheDocument()
    expect(screen.queryByLabelText("Finding summary")).toBeNull()
  })
})
