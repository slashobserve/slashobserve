import { describe, expect, test } from "vitest"

import {
  allEventTypes,
  eventTypeGroups,
  eventTypeLabels,
  eventTypeSummary,
  toggleEventType,
  toggleGroup,
} from "./event-types"

describe("eventTypeGroups", () => {
  test("groups every type once, by its prefix", () => {
    expect(eventTypeGroups.map((group) => group.prefix)).toEqual([
      "dashboard",
      "annotation",
      "finding",
      "thread",
      "run",
    ])
    expect(eventTypeGroups.flatMap((group) => group.types)).toEqual(
      allEventTypes
    )
    for (const group of eventTypeGroups) {
      for (const type of group.types)
        expect(type.startsWith(group.prefix + ".")).toBe(true)
    }
  })

  test("names every group", () => {
    expect(eventTypeGroups.map((group) => group.label)).toEqual([
      "Dashboards",
      "Annotations",
      "Findings",
      "Comment threads",
      "Agent runs",
    ])
  })

  test("has a label for every type", () => {
    expect(allEventTypes).toHaveLength(21)
    for (const type of allEventTypes) expect(eventTypeLabels[type]).not.toBe("")
  })
})

describe("toggleEventType", () => {
  test("adds in catalog order and removes", () => {
    expect(toggleEventType(["run.failed"], "dashboard.created", true)).toEqual([
      "dashboard.created",
      "run.failed",
    ])
    expect(
      toggleEventType(["dashboard.created", "run.failed"], "run.failed", false)
    ).toEqual(["dashboard.created"])
  })

  test("adding a type twice keeps one", () => {
    expect(toggleEventType(["run.failed"], "run.failed", true)).toEqual([
      "run.failed",
    ])
  })
})

describe("toggleGroup", () => {
  const runs = eventTypeGroups.find((group) => group.prefix === "run")!

  test("selects a whole group and keeps the rest", () => {
    expect(toggleGroup(["dashboard.created"], runs, true)).toEqual([
      "dashboard.created",
      ...runs.types,
    ])
  })

  test("clears a whole group and keeps the rest", () => {
    expect(
      toggleGroup(["dashboard.created", ...runs.types], runs, false)
    ).toEqual(["dashboard.created"])
  })
})

describe("eventTypeSummary", () => {
  test("names one or two types", () => {
    expect(eventTypeSummary(["run.failed"])).toBe("Run failed")
    expect(eventTypeSummary(["run.failed", "run.canceled"])).toBe(
      "Run failed, Run canceled"
    )
  })

  test("counts three or more", () => {
    expect(
      eventTypeSummary(["run.failed", "run.canceled", "run.started"])
    ).toBe("3 event types")
  })

  test("says all events when every type is chosen", () => {
    expect(eventTypeSummary(allEventTypes)).toBe("All events")
  })
})
