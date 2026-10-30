import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import {
  actionHost,
  actionOutcome,
  actionProblem,
  actionRequest,
  disabledReason,
  latestOnlyReason,
  needsEditReason,
  outsideDashboardReason,
  parseActions,
  requestLine,
  type CanvasActionContext,
  type ElementAction,
} from "./actions"

const context: CanvasActionContext = {
  dashboardId: "d1",
  version: 4,
  latestVersion: 4,
  canRun: true,
}

const noField = () => null

function oneAction(raw: Record<string, unknown>): ElementAction {
  const [action] = parseActions(
    [{ title: "Restart", url: "https://ops.example.com/restart", ...raw }],
    noField
  )
  return action
}

describe("disabledReason", () => {
  it.each([
    ["the editor preview has no provider", null, outsideDashboardReason],
    ["a person without Edit", { ...context, canRun: false }, needsEditReason],
    ["a pinned older version", { ...context, version: 3 }, latestOnlyReason],
    ["Edit on the latest version", context, null],
  ])("%s", (_name, given, expected) => {
    expect(disabledReason(given)).toBe(expected)
  })

  it("names the missing permission before the old version", () => {
    expect(disabledReason({ ...context, canRun: false, version: 1 })).toBe(
      needsEditReason
    )
  })
})

describe("actionHost", () => {
  it.each([
    ["https://ops.example.com/restart?x=1", "ops.example.com"],
    ["https://Ops.Example.com:443/restart", "ops.example.com"],
    ["https://ops.example.com", "ops.example.com"],
    ["https://ops.example.com/a/${region}", "ops.example.com"],
  ])("takes the host of %s", (url, host) => {
    expect(actionHost(url)).toBe(host)
  })

  it.each([
    "http://ops.example.com/x",
    "ops.example.com/x",
    "https://user:pw@ops.example.com/x",
    "https://${host}/x",
    "",
    42,
    undefined,
  ])("has no host for %j", (url) => {
    expect(actionHost(url)).toBeNull()
  })
})

describe("parseActions", () => {
  it("keeps each action's index in the saved list even when one is dropped", () => {
    const actions = parseActions(
      [
        { title: "A", url: "https://a.example.com/x" },
        { title: "", url: "https://b.example.com/x" },
        { title: "C", url: "http://c.example.com/x" },
        { title: "D", url: "https://d.example.com/x", method: "DELETE" },
      ],
      noField
    )
    expect(actions.map((action) => [action.index, action.title])).toEqual([
      [0, "A"],
      [3, "D"],
    ])
  })

  it("uses the Spec default method and keeps a written one", () => {
    expect(oneAction({}).method).toBe("POST")
    expect(oneAction({ method: "PUT" }).method).toBe("PUT")
    expect(oneAction({ method: "TRACE" }).method).toBe("POST")
  })

  it("keeps the confirm text only when it is written", () => {
    expect(oneAction({}).confirm).toBeNull()
    expect(oneAction({ confirm: "" }).confirm).toBeNull()
    expect(oneAction({ confirm: "Restart checkout?" }).confirm).toBe(
      "Restart checkout?"
    )
  })

  it("collects variables and data fields from every template, ignoring built-ins", () => {
    const action = parseActions(
      [
        {
          title: "Page",
          url: "https://ops.example.com/${region}/restart",
          query: [{ name: "since", value: "${__from:date}" }],
          headers: [{ name: "X-Team", value: "$team" }],
          body: '{"pod":"${__data.fields.pod}","who":"${__user.email}","v":${__value.numeric},"env":"${env:json}"}',
        },
      ],
      (name) => (name === "pod" ? "checkout-0" : null)
    )[0]
    expect(action.variables.sort()).toEqual(["env", "region", "team"])
    expect(action.fields).toEqual({ pod: "checkout-0" })
  })

  it("leaves out a data field with no value or an oversized value", () => {
    const action = parseActions(
      [
        {
          title: "Page",
          url: "https://ops.example.com/${__data.fields.a}/${__data.fields.b}/${__data.fields.c}",
        },
      ],
      (name) => ({ a: "x", b: "y".repeat(1001), c: null })[name] ?? null
    )[0]
    expect(action.fields).toEqual({ a: "x" })
  })

  it("never carries the address, headers or body", () => {
    const action = oneAction({
      body: "secret-ish",
      headers: [{ name: "X-A", value: "b" }],
    })
    expect(JSON.stringify(action)).not.toContain("restart")
    expect(JSON.stringify(action)).not.toContain("secret-ish")
    expect(Object.keys(action).sort()).toEqual([
      "confirm",
      "fields",
      "host",
      "index",
      "method",
      "title",
      "variables",
    ])
  })

  it("returns nothing for a missing or malformed list", () => {
    expect(parseActions(undefined, noField)).toEqual([])
    expect(parseActions("nope", noField)).toEqual([])
    expect(parseActions([null, 3], noField)).toEqual([])
  })
})

describe("actionRequest", () => {
  const action: ElementAction = {
    index: 2,
    title: "Restart",
    method: "POST",
    host: "ops.example.com",
    confirm: null,
    variables: ["region", "missing", "pods"],
    fields: { pod: "checkout-0" },
  }

  it("posts the latest version, ids, referenced variables, absolute range, value and fields", () => {
    const body = actionRequest({
      context,
      panelId: "canvas1",
      elementId: "restart",
      value: { raw: "0.5", text: "50%", numeric: 0.5 },
      action,
      vars: {
        region: ["eu"],
        pods: ["$__all"],
        unrelated: ["secret-looking"],
      },
      fromMs: Date.UTC(2026, 9, 30, 10, 0, 0),
      toMs: Date.UTC(2026, 9, 30, 11, 0, 0),
    })
    expect(body).toEqual({
      version: 4,
      panel: "canvas1",
      element: "restart",
      action: 2,
      variables: { region: ["eu"], pods: ["All"] },
      from: "2026-10-30T10:00:00.000Z",
      to: "2026-10-30T11:00:00.000Z",
      value: { raw: "0.5", text: "50%", numeric: 0.5 },
      fields: { pod: "checkout-0" },
    })
  })

  it("sends no value when the element is not bound to one", () => {
    const body = actionRequest({
      context,
      panelId: "p",
      elementId: "e",
      value: null,
      action,
      vars: {},
      fromMs: 0,
      toMs: 1000,
    })
    expect(body.value).toBeNull()
    expect(body.variables).toEqual({})
  })
})

describe("actionProblem", () => {
  const api = (status: number, code: string) =>
    new ApiError(status, code, "server text that is never shown")

  it.each([
    [
      api(403, "outbound_domain_not_allowed"),
      "ops.example.com is not allowed on this server.",
      true,
    ],
    [
      api(409, "stale_version"),
      "The dashboard changed. Reload it to run this action.",
      false,
    ],
    [
      api(429, "too_many_actions"),
      "Wait for your other actions to finish.",
      false,
    ],
    [api(502, "action_failed"), "ops.example.com did not answer.", false],
    [
      api(404, "action_not_found"),
      "This action is no longer in the dashboard.",
      false,
    ],
    [
      api(400, "action_invalid"),
      "This action cannot be built from the dashboard's current values.",
      false,
    ],
    [api(403, "forbidden_role"), needsEditReason, false],
    [api(500, "internal"), "Could not run this action.", false],
    [
      new NetworkError("Failed to fetch"),
      "Could not reach SlashObserve. Check your connection.",
      false,
    ],
    [new Error("boom"), "Could not run this action.", false],
  ])("maps %o", (error, text, allowlist) => {
    expect(actionProblem(error, "ops.example.com")).toEqual({
      text,
      allowlist,
    })
  })
})

describe("actionOutcome", () => {
  it("reports a 2xx answer as success", () => {
    expect(actionOutcome("Restart", { status: 204, ok: true })).toEqual({
      tone: "success",
      text: "Ran Restart.",
    })
  })

  it("reports any other answer with its status in the error tone", () => {
    expect(actionOutcome("Restart", { status: 302, ok: false })).toEqual({
      tone: "error",
      text: "Restart answered 302.",
    })
  })
})

describe("requestLine", () => {
  it("names the method and host only", () => {
    expect(requestLine({ ...oneAction({ method: "DELETE" }) })).toBe(
      "DELETE to ops.example.com"
    )
  })
})
