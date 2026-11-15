import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import {
  adminOnlyLinkText,
  linkFailureText,
  looksLikeLink,
} from "./grafana-link"

describe("looksLikeLink", () => {
  it.each([
    ["https://grafana.example.com/d/abc123/checkout?orgId=1", true],
    ["  http://grafana.local:3000/d/abc  ", true],
    ["HTTPS://grafana.example.com/d/abc", true],
    ['{ "title": "https://grafana.example.com" }', false],
    [
      "https://grafana.example.com/d/abc\nhttps://grafana.example.com/d/def",
      false,
    ],
    ["https://grafana.example.com/d/a b", false],
    ["grafana.example.com/d/abc", false],
    ["ftp://grafana.example.com/d/abc", false],
    ["", false],
  ])("reads %j as a link: %s", (text, expected) => {
    expect(looksLikeLink(text)).toBe(expected)
  })
})

describe("linkFailureText", () => {
  const failure = (status: number, code: string, message = "Server text") =>
    new ApiError(status, code, message)

  it("tells a role that is too low that only Admins import from a link", () => {
    expect(linkFailureText(failure(403, "forbidden_role"))).toBe(
      adminOnlyLinkText
    )
  })

  it("shows the server's words for a link it cannot use", () => {
    expect(
      linkFailureText(
        failure(404, "no_connection", "No connection stores this address")
      )
    ).toBe("No connection stores this address.")
    expect(
      linkFailureText(
        failure(422, "not_a_dashboard_link", "Not a dashboard link.")
      )
    ).toBe("Not a dashboard link.")
    expect(
      linkFailureText(
        failure(403, "outbound_domain_not_allowed", "Host not allowed")
      )
    ).toBe("Host not allowed.")
  })

  it("says Grafana did not answer", () => {
    expect(linkFailureText(failure(502, "grafana_unavailable"))).toBe(
      "Grafana did not answer. Try again, or paste the JSON."
    )
  })

  it("falls back for any other failure", () => {
    const text =
      "Could not fetch the dashboard from that link. Try again, or paste the JSON."
    expect(linkFailureText(failure(500, "internal"))).toBe(text)
    expect(linkFailureText(new Error("offline"))).toBe(text)
  })
})
