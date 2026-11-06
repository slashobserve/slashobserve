import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import {
  connectionProblems,
  grafanaProblemText,
  trimConnection,
} from "./connection-problems"

const valid = {
  name: "Production",
  url: "https://grafana.example.com",
  token: "glsa_abc",
}

describe("connectionProblems", () => {
  it("accepts a complete connection", () => {
    expect(connectionProblems(valid)).toEqual({})
  })

  it("reports every empty field", () => {
    expect(
      Object.keys(connectionProblems({ name: " ", url: "", token: "\t" }))
    ).toEqual(["name", "url", "token"])
  })

  it.each([
    ["a name over 100 characters", { name: "n".repeat(101) }, "name"],
    [
      "a URL over 2048 bytes",
      { url: `https://g.example.com/${"a".repeat(2030)}` },
      "url",
    ],
    ["a URL that does not parse", { url: "grafana.example.com" }, "url"],
    ["an ftp URL", { url: "ftp://x" }, "url"],
    ["a token over 4096 characters", { token: "t".repeat(4097) }, "token"],
  ])("rejects %s", (_name, change, field) => {
    expect(Object.keys(connectionProblems({ ...valid, ...change }))).toEqual([
      field,
    ])
  })

  it.each([
    ["exactly 100 name characters", { name: "n".repeat(100) }],
    ["an http URL with a port", { url: "http://grafana.internal:3000" }],
    ["exactly 4096 token characters", { token: "t".repeat(4096) }],
  ])("accepts %s", (_name, change) => {
    expect(connectionProblems({ ...valid, ...change })).toEqual({})
  })
})

describe("trimConnection", () => {
  it("trims every field", () => {
    expect(
      trimConnection({
        name: " a ",
        url: " https://g.example.com ",
        token: " t ",
      })
    ).toEqual({ name: "a", url: "https://g.example.com", token: "t" })
  })
})

describe("grafanaProblemText", () => {
  it.each([
    [
      "a missing secret key",
      new ApiError(
        409,
        "secret_key_missing",
        "SLASHOBSERVE_SECRET_KEY is not set"
      ),
      "Set SLASHOBSERVE_SECRET_KEY on the server and restart.",
    ],
    [
      "a locked allowlist",
      new ApiError(
        403,
        "outbound_domain_not_allowed",
        "grafana.example.com is not on the outbound allowlist."
      ),
      "grafana.example.com is not on the outbound allowlist.",
    ],
  ])("explains %s", (_name, error, want) => {
    expect(grafanaProblemText(error)).toBe(want)
  })
})
