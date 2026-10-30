import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { saveProblemFrom } from "./save-problem"

const heading = "The server rejected this:"

describe("saveProblemFrom", () => {
  it("reports a version conflict with the latest version", () => {
    const error = new ApiError(
      409,
      "version_conflict",
      "the dashboard changed",
      [],
      7
    )
    expect(saveProblemFrom(error, heading)).toEqual({
      kind: "conflict",
      latest: 7,
    })
  })

  it("treats a conflict without a latest version as a failure", () => {
    const error = new ApiError(409, "version_conflict", "the dashboard changed")
    expect(saveProblemFrom(error, heading)).toEqual({
      kind: "failed",
      message: "the dashboard changed",
    })
  })

  it("reports a failure when every issue is transient", () => {
    const error = new ApiError(422, "invalid_spec", "invalid", [
      { path: "/panels/0", code: "datasource_unavailable", message: "down" },
      { path: "/panels/1", code: "query_timeout", message: "slow" },
    ])
    expect(saveProblemFrom(error, heading)).toEqual({
      kind: "failed",
      message: "The panel queries could not be checked right now.",
    })
  })

  it("lists the issues of a rejected Spec under the given heading", () => {
    const error = new ApiError(422, "invalid_spec", "invalid", [
      { path: "/title", code: "invalid", message: "title is too long" },
    ])
    expect(saveProblemFrom(error, heading)).toEqual({
      kind: "invalid",
      heading,
      lines: ["title is too long"],
    })
  })

  it("falls back to the message when a 422 has no issues", () => {
    const error = new ApiError(422, "invalid_spec", "bad spec")
    expect(saveProblemFrom(error, heading)).toEqual({
      kind: "invalid",
      heading,
      lines: ["bad spec"],
    })
  })

  it("reports other API errors by their message", () => {
    const error = new ApiError(500, "internal", "server broke")
    expect(saveProblemFrom(error, heading)).toEqual({
      kind: "failed",
      message: "server broke",
    })
  })

  it("reports a plain Error by its message and anything else generically", () => {
    expect(saveProblemFrom(new Error("x"), heading)).toEqual({
      kind: "failed",
      message: "x",
    })
    expect(saveProblemFrom("boom", heading)).toEqual({
      kind: "failed",
      message: "The save failed.",
    })
  })
})
