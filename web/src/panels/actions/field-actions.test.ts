import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { ResolvedLink } from "../field-config"

import type { ActionContext } from "./actions"
import {
  fieldActionRequest,
  fieldActions,
  fieldActionValue,
  oneClickTarget,
  type FieldAction,
} from "./field-actions"

const frame: WireFrame = {
  ref: "A",
  fields: [
    { name: "Time", type: "time", values: [1, 2] },
    { name: "pod", type: "string", values: ["checkout-0", "checkout-1"] },
    { name: "Value", type: "number", values: [3, 4] },
  ],
}

const scope = {
  vars: { region: ["eu"] },
  row: { frame, index: 1 },
  value: 4,
  valueText: "4 req/s",
}

const restart = {
  title: "Restart",
  url: "https://ops.example.com/${__data.fields.pod}/restart",
}

const context: ActionContext = {
  dashboardId: "d1",
  version: 7,
  latestVersion: 7,
  canRun: true,
}

const link = (oneClick?: true): ResolvedLink => ({
  title: "Docs",
  href: "https://docs.example.com",
  newTab: false,
  ...(oneClick ? { oneClick } : {}),
})

function action(extra: Partial<FieldAction> = {}): FieldAction {
  return {
    index: 0,
    title: "Restart",
    method: "POST",
    host: "ops.example.com",
    confirm: null,
    variables: [],
    inputs: [],
    fields: {},
    color: null,
    oneClick: false,
    override: null,
    ...extra,
  }
}

describe("fieldActions", () => {
  it("reads a data field from the clicked row and carries the override the actions came from", () => {
    const [parsed] = fieldActions(
      { actions: [restart], actionsOverride: 2 },
      scope
    )
    expect(parsed.fields).toEqual({ pod: "checkout-1" })
    expect(parsed.override).toBe(2)
    expect(parsed.title).toBe("Restart")
  })

  it("has no data field when the scope has no row", () => {
    const [parsed] = fieldActions(
      { actions: [restart], actionsOverride: null },
      { vars: {} }
    )
    expect(parsed.fields).toEqual({})
    expect(parsed.override).toBeNull()
  })

  it("keeps each action's index in the list the server will read", () => {
    const parsed = fieldActions(
      {
        actions: [{ title: "", url: "https://a.example.com" }, restart],
        actionsOverride: null,
      },
      scope
    )
    expect(parsed.map((item) => item.index)).toEqual([1])
  })
})

describe("fieldActionValue", () => {
  it.each([
    [
      { value: 4, valueText: "4 req/s" },
      { raw: "4", text: "4 req/s", numeric: 4 },
    ],
    [
      { value: "up", valueText: "Up" },
      { raw: "up", text: "Up", numeric: null },
    ],
    [
      { value: null, valueText: "No data" },
      { raw: "", text: "No data", numeric: null },
    ],
    [{ value: Number.NaN }, { raw: "NaN", text: "NaN", numeric: null }],
    [{}, null],
  ])("turns %j into %j", (given, expected) => {
    expect(fieldActionValue(given)).toEqual(expected)
  })
})

describe("fieldActionRequest", () => {
  const range = {
    fromMs: Date.UTC(2026, 9, 30, 10),
    toMs: Date.UTC(2026, 9, 30, 11),
  }

  it("names the override and the clicked value", () => {
    const body = fieldActionRequest({
      context,
      panelId: "table1",
      action: action({ index: 1, override: 3, fields: { pod: "checkout-1" } }),
      scope,
      ...range,
    })
    expect(body).toMatchObject({
      version: 7,
      panel: "table1",
      override: 3,
      action: 1,
      value: { raw: "4", text: "4 req/s", numeric: 4 },
      fields: { pod: "checkout-1" },
    })
    expect(body).not.toHaveProperty("element")
  })

  it("sends no override for the panel's own actions", () => {
    const body = fieldActionRequest({
      context,
      panelId: "table1",
      action: action(),
      scope,
      ...range,
    })
    expect(body).not.toHaveProperty("override")
  })
})

describe("oneClickTarget", () => {
  it("is the only link when it has oneClick", () => {
    expect(oneClickTarget([link(true)], [])).toEqual({
      kind: "link",
      link: link(true),
    })
  })

  it("is the only action when it has oneClick", () => {
    const only = action({ oneClick: true })
    expect(oneClickTarget([], [only])).toEqual({ kind: "action", action: only })
  })

  it("is none when the only link or action lacks oneClick", () => {
    expect(oneClickTarget([link()], [])).toBeNull()
    expect(oneClickTarget([], [action()])).toBeNull()
  })

  it("is none with one of each, whichever has oneClick", () => {
    expect(
      oneClickTarget([link(true)], [action({ oneClick: true })])
    ).toBeNull()
  })

  it("is none with several actions or none at all", () => {
    const one = action({ oneClick: true })
    expect(oneClickTarget([], [one, one])).toBeNull()
    expect(oneClickTarget([], [])).toBeNull()
  })
})
