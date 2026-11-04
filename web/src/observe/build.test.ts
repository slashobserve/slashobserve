import { describe, expect, it } from "vitest"

import {
  buildStatusText,
  cancelErrorText,
  canContinueBuild,
  canSaveBuild,
  continueErrorText,
  continueNoteError,
  newPanelIds,
  saveBuildErrorText,
  type BuildProgress,
} from "./build"

function progress(overrides: Partial<BuildProgress> = {}): BuildProgress {
  return {
    saves: 3,
    panels: 12,
    planned_panels: 40,
    title: "Checkout",
    updated_at: "2026-11-03T10:00:00Z",
    ...overrides,
  }
}

describe("buildStatusText", () => {
  it.each([
    ["queued", null, null, "Waiting for the agent"],
    ["running", null, null, "Waiting for the agent"],
    ["running", progress(), null, "Building, 12 of 40 panels"],
    ["running", progress({ panels: 40 }), null, "Building, 40 of 40 panels"],
    ["running", progress({ panels: 45 }), null, "Building, 45 panels"],
    [
      "running",
      progress({ planned_panels: 0, panels: 12 }),
      null,
      "Building, 12 panels",
    ],
    [
      "running",
      progress({ planned_panels: 0, panels: 1 }),
      null,
      "Building, 1 panel",
    ],
    [
      "failed",
      progress({ panels: 37, planned_panels: 114 }),
      "no_progress",
      "Stopped at 37 of 114 panels",
    ],
    [
      "failed",
      progress({ panels: 37, planned_panels: 114 }),
      "canceled",
      "Canceled at 37 of 114 panels",
    ],
    [
      "failed",
      progress({ planned_panels: 0, panels: 41 }),
      "no_progress",
      "Stopped with 41 panels",
    ],
    [
      "failed",
      progress({ planned_panels: 0, panels: 1 }),
      "canceled",
      "Canceled with 1 panel",
    ],
    [
      "failed",
      progress({ panels: 45, planned_panels: 40 }),
      "no_progress",
      "Stopped with 45 panels",
    ],
    [
      "failed",
      progress({ panels: 0 }),
      "no_progress",
      "Stopped before anything was built",
    ],
    ["failed", null, "provider_error", "Stopped before anything was built"],
    ["succeeded", null, null, "Finished"],
  ] as const)(
    "%s with %j and %s reads %s",
    (status, build, failureCode, text) => {
      expect(buildStatusText({ status, build, failureCode })).toBe(text)
    }
  )
})

describe("buildStatusText queue, cancel and continue", () => {
  it.each([
    ["queued", null, null, 3, false, false, "Queued, position 3"],
    ["queued", null, null, 1, false, false, "Queued, position 1"],
    ["queued", null, null, null, false, false, "Waiting for the agent"],
    ["queued", null, null, 2, true, false, "Canceling"],
    ["running", progress(), null, null, true, false, "Canceling"],
    ["running", null, null, null, true, false, "Canceling"],
    ["running", progress(), null, 4, false, false, "Building, 12 of 40 panels"],
    [
      "failed",
      progress(),
      "no_progress",
      null,
      false,
      true,
      "Continued in a new run",
    ],
    [
      "failed",
      progress(),
      "canceled",
      null,
      true,
      false,
      "Canceled at 12 of 40 panels",
    ],
  ] as const)(
    "%s with %j, position %s, canceling %s, continued %s reads %s",
    (status, build, failureCode, queuePosition, canceling, continued, text) => {
      expect(
        buildStatusText({
          status,
          build,
          failureCode,
          queuePosition,
          canceling,
          continued,
        })
      ).toBe(text)
    }
  )
})

describe("newPanelIds", () => {
  const before = {
    panels: [
      { id: "row_a", panels: [{ id: "a1" }, { id: "a2" }] },
      { id: "solo" },
    ],
  }

  it("lists nothing when nothing was added", () => {
    expect(newPanelIds(before, before).size).toBe(0)
  })

  it("lists a new row and its children", () => {
    const next = {
      panels: [...before.panels, { id: "row_b", panels: [{ id: "b1" }] }],
    }
    expect([...newPanelIds(before, next)].sort()).toEqual(["b1", "row_b"])
  })

  it("lists children added to an existing row", () => {
    const next = {
      panels: [
        { id: "row_a", panels: [{ id: "a1" }, { id: "a2" }, { id: "a3" }] },
        { id: "solo" },
      ],
    }
    expect([...newPanelIds(before, next)]).toEqual(["a3"])
  })

  it("lists a panel that moved into a row as unchanged", () => {
    const next = {
      panels: [
        { id: "row_a", panels: [{ id: "a1" }, { id: "a2" }, { id: "solo" }] },
      ],
    }
    expect(newPanelIds(before, next).size).toBe(0)
  })

  it("ignores panels that were removed", () => {
    expect(newPanelIds(before, { panels: [{ id: "solo" }] }).size).toBe(0)
  })
})

describe("canSaveBuild", () => {
  const ok = {
    status: "failed" as const,
    build: progress(),
    buildSavedAt: null,
    isRequester: true,
  }

  it("allows a failed request with panels for its requester", () => {
    expect(canSaveBuild(ok)).toBe(true)
  })

  it.each([
    ["the request is running", { status: "running" as const }],
    ["the request succeeded", { status: "succeeded" as const }],
    ["there is no build", { build: null }],
    ["the build has no panels", { build: progress({ panels: 0 }) }],
    ["the build was already saved", { buildSavedAt: "2026-11-03T10:00:00Z" }],
    ["the caller is not the requester", { isRequester: false }],
  ])("refuses when %s", (_name, change) => {
    expect(canSaveBuild({ ...ok, ...change })).toBe(false)
  })
})

describe("saveBuildErrorText", () => {
  it.each([
    [
      "version_conflict",
      "Someone saved a newer version after this build started, so it can't be saved. Open the dashboard and ask again.",
    ],
    ["forbidden", "You need Editor access to save it."],
    ["forbidden_role", "You need Editor access to save it."],
    ["build_empty", "Nothing was built yet."],
    ["build_running", "The agent is still working."],
    ["internal", "The agent is still working."],
  ])("explains %s", (code, text) => {
    expect(saveBuildErrorText(code, "the agent is still working")).toBe(text)
  })

  it("falls back when there is no message", () => {
    expect(saveBuildErrorText("internal")).toBe("The build could not be saved.")
  })
})

describe("canContinueBuild", () => {
  const ok = {
    status: "failed" as const,
    build: progress(),
    buildSavedAt: null,
    continuedBy: null,
    isRequester: true,
  }

  it("allows a stopped build with panels for its requester", () => {
    expect(canContinueBuild(ok)).toBe(true)
  })

  it.each([
    ["the request is running", { status: "running" as const }],
    ["the request succeeded", { status: "succeeded" as const }],
    ["there is no build", { build: null }],
    ["the build has no panels", { build: progress({ panels: 0 }) }],
    ["the build was already saved", { buildSavedAt: "2026-11-03T10:00:00Z" }],
    ["the build was continued", { continuedBy: "0b7a3c1e" }],
    ["the caller is not the requester", { isRequester: false }],
  ])("refuses when %s", (_name, change) => {
    expect(canContinueBuild({ ...ok, ...change })).toBe(false)
  })
})

describe("continueNoteError", () => {
  it.each([
    ["", null],
    ["   \n ", null],
    ["a".repeat(4000), null],
    [`  ${"a".repeat(4000)}  `, null],
    ["a".repeat(4001), "Use at most 4000 characters."],
    ["\u{1F600}".repeat(4000), null],
    ["\u{1F600}".repeat(4001), "Use at most 4000 characters."],
  ])("checks a note of %#", (raw, error) => {
    expect(continueNoteError(raw)).toBe(error)
  })
})

describe("cancelErrorText", () => {
  it.each([
    ["forbidden", "Only the person who asked can do this."],
    ["request_finished", "The run already finished."],
    ["internal", "The server broke."],
  ])("explains %s", (code, text) => {
    expect(cancelErrorText(code, "the server broke")).toBe(text)
  })

  it("falls back when there is no message", () => {
    expect(cancelErrorText("internal")).toBe("The run could not be canceled.")
  })
})

describe("continueErrorText", () => {
  it.each([
    [
      "version_conflict",
      "Someone saved a newer version after this build started, so it can't be continued. Open the dashboard and ask again.",
    ],
    ["forbidden", "Only the person who asked can do this."],
    ["not_continuable", "This build can no longer be continued."],
    ["invalid_request", "Note must be short."],
  ])("explains %s", (code, text) => {
    expect(continueErrorText(code, "note must be short")).toBe(text)
  })

  it("falls back when there is no message", () => {
    expect(continueErrorText("internal")).toBe(
      "The build could not be continued."
    )
  })
})
