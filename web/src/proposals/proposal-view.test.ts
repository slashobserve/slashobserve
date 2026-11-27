import { describe, expect, it } from "vitest"

import type { MergeConflict, Proposal, ProposalSummary } from "./queries"
import {
  changeRows,
  choicesFrom,
  conflictText,
  dashboardLinkOf,
  droppedText,
  mergeHint,
  stateLine,
  unitKey,
} from "./proposal-view"

const ada = {
  kind: "user" as const,
  id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
  name: "Ada",
}

const adaActor = {
  ...ada,
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function summary(over: Partial<ProposalSummary> = {}): ProposalSummary {
  return {
    id: "p1",
    dashboard_id: "d1",
    state: "open",
    title: "Add latency",
    revision: 1,
    base_version: 3,
    summary: "",
    author: ada,
    actor: adaActor,
    origin: "web",
    origin_client: null,
    run_id: null,
    created_at: "2026-11-11T10:00:00Z",
    updated_at: "2026-11-11T10:00:00Z",
    closed_at: null,
    closed_by: null,
    close_reason: null,
    merged_version: null,
    behind: false,
    latest_version: 3,
    allowed_actions: ["merge", "discard"],
    ...over,
  }
}

function conflict(over: Partial<MergeConflict> = {}): MergeConflict {
  return {
    kind: "panel",
    id: "p-errors",
    reason: "both_changed",
    message: "Both changed this panel.",
    ...over,
  }
}

function proposal(over: Partial<Proposal> = {}): Proposal {
  return {
    ...summary(),
    spec: {
      specVersion: "0",
      panels: [
        { id: "p-errors", title: "Errors" },
        { id: "p-latency", title: "Latency" },
      ],
    },
    changes: [],
    diff: [],
    conflicts: [],
    mergeability: "current",
    ...over,
  }
}

describe("changeRows", () => {
  it("sorts panels, then variables, then settings, each by label", () => {
    const rows = changeRows(
      proposal({
        changes: [
          { kind: "setting", id: "refresh", change: "changed" },
          { kind: "variable", id: "env", change: "added" },
          { kind: "panel", id: "p-latency", change: "added" },
          { kind: "panel", id: "p-errors", change: "moved" },
        ],
      }),
      new Map()
    )
    expect(rows.map((row) => [row.key, row.label, row.change])).toEqual([
      ["panel:p-errors", "Errors", "moved"],
      ["panel:p-latency", "Latency", "added"],
      ["variable:env", "Variable env", "added"],
      ["setting:refresh", "Dashboard settings", "changed"],
    ])
  })

  it("names a removed panel from the dashboard's titles and falls back to its id", () => {
    const rows = changeRows(
      proposal({
        changes: [
          { kind: "panel", id: "p-old", change: "removed" },
          { kind: "panel", id: "p-gone", change: "removed" },
        ],
      }),
      new Map([["p-old", "Old traffic"]])
    )
    expect(rows.map((row) => row.label)).toEqual(["Old traffic", "p-gone"])
  })

  it("attaches a conflict to the row it belongs to", () => {
    const clash = conflict()
    const rows = changeRows(
      proposal({
        changes: [{ kind: "panel", id: "p-errors", change: "changed" }],
        conflicts: [clash],
      }),
      new Map()
    )
    expect(rows).toHaveLength(1)
    expect(rows[0]?.conflict).toBe(clash)
    expect(rows[0]?.change).toBe("changed")
  })

  it("adds a row for a conflict the changes do not list", () => {
    const clash = conflict({ kind: "variable", id: "env", reason: "invalid" })
    const rows = changeRows(proposal({ conflicts: [clash] }), new Map())
    expect(rows).toEqual([
      {
        key: "variable:env",
        kind: "variable",
        id: "env",
        label: "Variable env",
        change: "conflict",
        conflict: clash,
        dashboardTitle: null,
        proposalTitle: null,
      },
    ])
  })

  it("carries both sides' titles for a panel conflict", () => {
    const clash = conflict({ id: "p-errors" })
    const rows = changeRows(
      proposal({
        changes: [{ kind: "panel", id: "p-errors", change: "changed" }],
        conflicts: [clash],
      }),
      new Map([["p-errors", "Errors (admin edit)"]])
    )
    expect(rows[0]).toMatchObject({
      dashboardTitle: "Errors (admin edit)",
      proposalTitle: "Errors",
    })
  })

  it("gives a row without a conflict a null conflict", () => {
    const rows = changeRows(
      proposal({
        changes: [{ kind: "panel", id: "p-latency", change: "added" }],
      }),
      new Map()
    )
    expect(rows[0]?.conflict).toBeNull()
  })
})

describe("choicesFrom", () => {
  const both = conflict()
  const removed = conflict({
    id: "p-latency",
    reason: "changed_and_removed",
  })
  const invalid = conflict({ kind: "setting", id: "time", reason: "invalid" })

  it("turns every pick into a choice", () => {
    expect(
      choicesFrom([both, removed], {
        [unitKey(both)]: "theirs",
        [unitKey(removed)]: "ours",
      })
    ).toEqual({
      choices: [
        { kind: "panel", id: "p-errors", take: "theirs" },
        { kind: "panel", id: "p-latency", take: "ours" },
      ],
      missing: [],
    })
  })

  it("lists the conflicts without a pick as missing", () => {
    expect(choicesFrom([both, removed], { "panel:p-errors": "ours" })).toEqual({
      choices: [{ kind: "panel", id: "p-errors", take: "ours" }],
      missing: ["panel:p-latency"],
    })
  })

  it("never offers an invalid conflict as a choice, even with a stale pick", () => {
    expect(choicesFrom([invalid], { "setting:time": "theirs" })).toEqual({
      choices: [],
      missing: ["setting:time"],
    })
  })

  it("has nothing missing without conflicts", () => {
    expect(choicesFrom([], { "panel:p-old": "ours" })).toEqual({
      choices: [],
      missing: [],
    })
  })
})

describe("droppedText", () => {
  it("names each unit whose dashboard side was kept", () => {
    const conflicts = [
      conflict(),
      conflict({ kind: "variable", id: "env" }),
      conflict({ kind: "setting", id: "refresh" }),
    ]
    expect(
      droppedText(proposal({ conflicts }), {
        "panel:p-errors": "ours",
        "variable:env": "ours",
        "setting:refresh": "ours",
      })
    ).toBe(
      'The proposed change to panel "Errors", variable "env", the refresh setting was dropped; the dashboard keeps its own.'
    )
  })

  it("says nothing when every conflict took the proposal's side", () => {
    expect(
      droppedText(proposal({ conflicts: [conflict()] }), {
        "panel:p-errors": "theirs",
      })
    ).toBeNull()
  })
})

describe("mergeHint", () => {
  it("says a blocked proposal cannot be merged, whatever was chosen", () => {
    expect(mergeHint({ mergeability: "blocked" }, [])).toBe(
      "Some changes cannot be merged; discard this proposal or ask its author to fix it"
    )
    expect(mergeHint({ mergeability: "blocked" }, ["panel:p1"])).toBe(
      "Some changes cannot be merged; discard this proposal or ask its author to fix it"
    )
  })

  it("asks for a side while a conflict has none", () => {
    expect(
      mergeHint({ mergeability: "behind_conflicting" }, ["panel:p1"])
    ).toBe("Choose a side for each conflict first")
  })

  it("says nothing when the merge can go ahead", () => {
    expect(mergeHint({ mergeability: "current" }, [])).toBeNull()
    expect(mergeHint({ mergeability: "behind_clean" }, [])).toBeNull()
    expect(mergeHint({ mergeability: "behind_conflicting" }, [])).toBeNull()
  })
})

describe("stateLine", () => {
  it.each([
    [summary(), "Open, based on version 3"],
    [
      summary({ behind: true, latest_version: 5 }),
      "Open, based on version 3. The dashboard is now at version 5.",
    ],
    [
      summary({
        state: "merged",
        merged_version: 6,
        closed_by: { ...ada, name: "Grace" },
      }),
      "Merged as version 6 by Grace",
    ],
    [
      summary({ state: "merged", merged_version: null, closed_by: ada }),
      "Merged without a new version; nothing from this proposal was added to the dashboard",
    ],
    [
      summary({ state: "discarded", closed_by: { ...ada, name: "Grace" } }),
      "Discarded by Grace",
    ],
  ])("reads %#", (item, expected) => {
    expect(stateLine(item)).toBe(expected)
  })
})

describe("conflictText", () => {
  it("names the panel by its title, not its id", () => {
    expect(conflictText(conflict(), "Request rate")).toBe(
      'Panel "Request rate" was changed on the dashboard and in this proposal.'
    )
    expect(
      conflictText(conflict({ reason: "changed_and_removed" }), "Request rate")
    ).toBe(
      'Panel "Request rate" was removed on one side and changed on the other.'
    )
  })

  it("keeps the server's text for a change that cannot merge", () => {
    expect(
      conflictText(conflict({ reason: "invalid", message: "bad query" }), "x")
    ).toBe("bad query")
  })
})

describe("dashboardLinkOf", () => {
  it.each([
    [summary(), 3, "Open the dashboard at version 3"],
    [
      summary({ state: "discarded", closed_by: ada }),
      3,
      "Open the dashboard at version 3",
    ],
    [
      summary({ state: "merged", merged_version: 8, closed_by: ada }),
      8,
      "Open the dashboard at version 8",
    ],
    [
      summary({ state: "merged", merged_version: null, closed_by: ada }),
      null,
      "Open the dashboard",
    ],
  ])("points %#", (item, version, label) => {
    expect(dashboardLinkOf(item)).toEqual({ version, label })
  })
})
