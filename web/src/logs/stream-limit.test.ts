import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import { streamLimit, streamLimitText } from "./stream-limit"

const seriesMessage =
  "query limit exceeded: series limit exceeded: more than 200 series"

describe("streamLimit", () => {
  it("reads the number from the server's series limit message", () => {
    expect(
      streamLimit(new ApiError(422, "limit_exceeded", seriesMessage))
    ).toBe(200)
  })

  it("ignores another limit_exceeded message", () => {
    expect(
      streamLimit(
        new ApiError(422, "limit_exceeded", "row limit exceeded: 10000 rows")
      )
    ).toBeNull()
  })

  it("ignores the message under another code", () => {
    expect(
      streamLimit(new ApiError(422, "query_invalid", seriesMessage))
    ).toBeNull()
  })

  it("ignores a network error", () => {
    expect(streamLimit(new NetworkError(seriesMessage))).toBeNull()
  })
})

describe("streamLimitText", () => {
  it("groups thousands", () => {
    expect(streamLimitText(1200)).toBe(
      "More than 1,200 streams match. Narrow the selector."
    )
  })
})
