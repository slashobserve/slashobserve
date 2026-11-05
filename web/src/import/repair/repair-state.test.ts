import { describe, expect, it } from "vitest"

import {
  anyActive,
  outcomeLines,
  repairState,
  unverifiedText,
  type RepairItem,
} from "./repair-state"

function item(
  repair: RepairItem["repair"],
  counts: Partial<
    Pick<
      RepairItem,
      "unverified_panels" | "unverified_variables" | "placeholder_panels"
    >
  > = {}
): RepairItem {
  return {
    id: "d1",
    title: "Checkout",
    url: "https://observe.example.com/d/d1/checkout",
    folder: { id: "f1", title: "Imported", general: false, personal: false },
    unverified_panels: 1,
    unverified_variables: 0,
    placeholder_panels: 0,
    last_import_at: null,
    repair,
    ...counts,
  }
}

function repair(status: NonNullable<RepairItem["repair"]>["status"]) {
  return {
    id: "r1",
    status,
    finished_at:
      status === "succeeded" || status === "failed"
        ? "2026-11-06T10:00:00Z"
        : null,
  }
}

describe("repairState", () => {
  it.each([
    [
      "no repair yet",
      item(null),
      { tone: null, label: null, canRepair: true, reason: null },
    ],
    [
      "a queued repair",
      item(repair("queued")),
      {
        tone: "running",
        label: "Repair queued",
        canRepair: false,
        reason: "A repair is already queued or running",
      },
    ],
    [
      "a running repair",
      item(repair("running")),
      {
        tone: "running",
        label: "Repairing",
        canRepair: false,
        reason: "A repair is already queued or running",
      },
    ],
    [
      "a repair that succeeded but left failures",
      item(repair("succeeded")),
      {
        tone: "ok",
        label: "Repaired, still failing",
        canRepair: true,
        reason: null,
      },
    ],
    [
      "a failed repair",
      item(repair("failed")),
      {
        tone: "critical",
        label: "Repair failed",
        canRepair: true,
        reason: null,
      },
    ],
    [
      "only placeholders",
      item(null, { unverified_panels: 0, placeholder_panels: 2 }),
      {
        tone: null,
        label: null,
        canRepair: false,
        reason:
          "Placeholders are replaced by hand; the agent does not edit them",
      },
    ],
    [
      "only an unverified variable",
      item(null, { unverified_panels: 0, unverified_variables: 1 }),
      { tone: null, label: null, canRepair: true, reason: null },
    ],
  ])("%s", (_name, given, want) => {
    expect(repairState(given, true)).toEqual(want)
  })

  it("cannot repair without an agent", () => {
    expect(repairState(item(repair("failed")), false)).toEqual({
      tone: "critical",
      label: "Repair failed",
      canRepair: false,
      reason: "No agent is configured",
    })
  })
})

describe("anyActive", () => {
  it.each([
    ["no items", [], false],
    ["no repairs", [item(null)], false],
    [
      "finished repairs",
      [item(repair("succeeded")), item(repair("failed"))],
      false,
    ],
    ["a queued repair", [item(null), item(repair("queued"))], true],
    ["a running repair", [item(repair("running"))], true],
  ])("%s", (_name, items, want) => {
    expect(anyActive(items)).toBe(want)
  })
})

describe("unverifiedText", () => {
  it.each([
    [{ unverified_panels: 2, unverified_variables: 1 }, "2 panels, 1 variable"],
    [{ unverified_panels: 1, unverified_variables: 0 }, "1 panel"],
    [{ unverified_panels: 0, unverified_variables: 3 }, "3 variables"],
    [{ unverified_panels: 0, unverified_variables: 0 }, "None"],
  ])("describes %o", (counts, want) => {
    expect(unverifiedText(item(null, counts))).toBe(want)
  })
})

describe("outcomeLines", () => {
  function skipped(
    ...reasons: (
      | "active"
      | "nothing_to_repair"
      | "not_found"
      | "forbidden_role"
      | "queue_full"
    )[]
  ) {
    return reasons.map((reason, i) => ({ dashboard_id: `d${i}`, reason }))
  }
  const queued = (n: number) =>
    Array.from({ length: n }, (_, i) => ({
      dashboard_id: `q${i}`,
      request_id: `r${i}`,
    }))

  it.each([
    ["one queued", { queued: queued(1), skipped: [] }, ["Queued 1 repair"]],
    [
      "queued and skipped",
      { queued: queued(3), skipped: skipped("active", "nothing_to_repair") },
      [
        "Queued 3 repairs",
        "Skipped 2: already queued or running, or nothing the agent can repair",
      ],
    ],
    [
      "the requester cap",
      {
        queued: queued(0),
        skipped: skipped("queue_full", "queue_full", "active"),
      },
      [
        "No repairs queued",
        "Skipped 1: already queued or running, or nothing the agent can repair",
        "2 not queued yet: you can have 20 repairs queued or running. Run Repair all again when these finish.",
      ],
    ],
  ])("%s", (_name, outcome, want) => {
    expect(outcomeLines(outcome)).toEqual(want)
  })
})
