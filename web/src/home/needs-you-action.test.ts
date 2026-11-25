import { describe, expect, test } from "vitest"

import type { InboxItem } from "@/inbox/inbox-item"

import { needsYouAction } from "./needs-you-action"

const proposalId = "3f1c1f0e-6d57-4a0e-8d0e-6c5c1b1d9a01"
const observationId = "8a2b7c3d-1e4f-4b5a-9c6d-7e8f9a0b1c2d"
const requestId = "c4d5e6f7-0a1b-4c2d-8e3f-9a0b1c2d3e4f"

function item(
  reason: InboxItem["reason"],
  target: InboxItem["target"],
  data: Record<string, unknown> = {}
): InboxItem {
  return {
    seq: 1,
    type: "proposal.opened",
    target,
    dashboard_id: null,
    folder_id: null,
    data,
    reason,
  } as unknown as InboxItem
}

describe("needsYouAction", () => {
  test("Change role opens People for a request in General, where nothing can be shared", () => {
    const general = {
      ...item(
        "access_request",
        { kind: "access_request", id: requestId },
        { object: { kind: "dashboard", id: proposalId }, general: true }
      ),
      actor: { kind: "user", name: "Ada" },
    } as InboxItem
    expect(needsYouAction(general)).toMatchObject({
      kind: "link",
      label: "Change role",
      to: { to: "/people" },
    })
  })

  test("Share opens the Share pane of the dashboard someone asked about", () => {
    expect(
      needsYouAction(
        item(
          "access_request",
          { kind: "access_request", id: requestId },
          { object: { kind: "dashboard", id: proposalId } }
        )
      )
    ).toEqual({
      kind: "link",
      label: "Share",
      to: { to: `/d/${proposalId}`, search: { share: "1" } },
    })
  })

  test("Review opens a proposal", () => {
    expect(
      needsYouAction(item("proposal", { kind: "proposal", id: proposalId }))
    ).toEqual({
      kind: "link",
      label: "Review",
      to: { to: `/proposals/${proposalId}` },
    })
  })

  test("Open shows a closed proposal", () => {
    expect(
      needsYouAction(
        item("proposal_closed", { kind: "proposal", id: proposalId })
      )
    ).toEqual({
      kind: "link",
      label: "Open",
      to: { to: `/proposals/${proposalId}` },
    })
  })

  test("Review opens the observation of a requested block", () => {
    const action = needsYouAction(
      item("requested_block", { kind: "observation", id: observationId })
    )
    expect(action).toMatchObject({ kind: "link", label: "Review" })
  })

  test("Reply opens the thread of a mention and of a reply", () => {
    const data = { object: { kind: "observation", id: observationId } }
    for (const reason of ["mention", "reply"] as const) {
      expect(
        needsYouAction(item(reason, { kind: "thread", id: "t1" }, data))
      ).toEqual({
        kind: "link",
        label: "Reply",
        to: { to: `/o/${observationId}`, search: { thread: "t1" } },
      })
    }
  })

  test("Try again retries the failed run the item is about", () => {
    expect(
      needsYouAction(item("run_failed", { kind: "run", id: requestId }))
    ).toEqual({ kind: "retry", requestId })
  })

  test("a failed item that is not about a run has no action", () => {
    expect(
      needsYouAction(item("run_failed", { kind: "dashboard", id: "d1" }))
    ).toBeNull()
  })

  test("Open opens a repaired dashboard", () => {
    const repaired = {
      ...item("repaired", { kind: "run", id: requestId }),
      type: "run.succeeded",
      dashboard_id: "d1",
    } as InboxItem
    expect(needsYouAction(repaired)).toMatchObject({
      kind: "link",
      label: "Open",
    })
  })

  test("an item with no reason has no action", () => {
    expect(
      needsYouAction(item(null, { kind: "proposal", id: proposalId }))
    ).toBeNull()
  })

  test("an item whose page is unknown has no action", () => {
    expect(
      needsYouAction(item("mention", { kind: "thread", id: "t1" }))
    ).toBeNull()
  })
})
