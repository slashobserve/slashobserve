import { describe, expect, test } from "vitest"

import { ApiError } from "@/api/errors"

import {
  disabledReasonText,
  isDomainNotAllowed,
  trimWebhook,
  webhookProblems,
  webhookProblemText,
} from "./webhook-problems"

const valid = {
  name: "Failed runs",
  url: "https://hooks.example.com/in",
  events: ["run.failed" as const],
}

describe("webhookProblems", () => {
  test("accepts a valid webhook", () => {
    expect(webhookProblems(valid)).toEqual({})
  })

  test("trims the name and address first", () => {
    expect(
      webhookProblems({
        ...valid,
        name: "  x  ",
        url: "  https://a.example.com ",
      })
    ).toEqual({})
    expect(trimWebhook({ ...valid, name: " x ", url: " u " })).toMatchObject({
      name: "x",
      url: "u",
    })
  })

  test("requires a name of at most 100 characters", () => {
    expect(webhookProblems({ ...valid, name: " " }).name).toBe("Enter a name.")
    expect(webhookProblems({ ...valid, name: "a".repeat(101) }).name).toMatch(
      /at most 100/
    )
    expect(
      webhookProblems({ ...valid, name: "é".repeat(100) }).name
    ).toBeUndefined()
  })

  test("requires an https address on port 443 without credentials or a fragment", () => {
    const url = (value: string) => webhookProblems({ ...valid, url: value }).url
    expect(url("")).toMatch(/Enter the address/)
    expect(url("hooks.example.com")).toMatch(/full https address/)
    expect(url("http://hooks.example.com")).toMatch(/https/)
    expect(url("https://hooks.example.com:8443/x")).toMatch(/port 443/)
    expect(url("https://user:pw@hooks.example.com")).toMatch(/user name/)
    expect(url("https://hooks.example.com/x#frag")).toMatch(/fragment/)
    expect(url("https://hooks.example.com:443/x")).toBeUndefined()
    expect(url("https://hooks.example.com/" + "a".repeat(2048))).toMatch(
      /at most 2048/
    )
  })

  test("requires at least one event", () => {
    expect(webhookProblems({ ...valid, events: [] }).events).toBe(
      "Choose at least one event."
    )
  })
})

describe("webhookProblemText", () => {
  test("explains the codes the server answers with", () => {
    expect(
      webhookProblemText(new ApiError(409, "secret_key_missing", "x"))
    ).toMatch(/SLASHOBSERVE_SECRET_KEY/)
    expect(
      webhookProblemText(new ApiError(409, "secret_unreadable", "x"))
    ).toMatch(/Rotate the secret/)
    expect(
      webhookProblemText(new ApiError(422, "too_many_webhooks", "x"))
    ).toMatch(/At most 20/)
    expect(
      webhookProblemText(new ApiError(400, "invalid_request", "Bad name."))
    ).toBe("Bad name.")
  })
})

describe("isDomainNotAllowed", () => {
  test("matches only outbound_domain_not_allowed", () => {
    expect(
      isDomainNotAllowed(new ApiError(403, "outbound_domain_not_allowed", "x"))
    ).toBe(true)
    expect(isDomainNotAllowed(new ApiError(403, "forbidden_role", "x"))).toBe(
      false
    )
    expect(isDomainNotAllowed(new Error("x"))).toBe(false)
  })
})

describe("disabledReasonText", () => {
  test("names each reason", () => {
    for (const reason of [
      "manual",
      "gone",
      "failing",
      "owner",
      "secret_unreadable",
    ]) {
      expect(disabledReasonText(reason)).not.toBe("Disabled")
    }
    expect(disabledReasonText("unknown")).toBe("Disabled")
    expect(disabledReasonText(null)).toBe("Disabled")
  })
})
