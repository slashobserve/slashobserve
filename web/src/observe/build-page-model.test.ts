import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { buildPageNotice, buildPageTitle } from "./build-page-model"

const notFound = new ApiError(404, "not_found", "not found")
const serverError = new ApiError(500, "internal", "boom")

describe("buildPageNotice", () => {
  it.each([
    [
      "request 404 with no request",
      notFound,
      false,
      null,
      "Only the person who asked can watch this build.",
    ],
    [
      "request 500 with no request",
      serverError,
      false,
      null,
      "This build could not be loaded.",
    ],
    [
      "request network failure with no request",
      new TypeError("fetch"),
      false,
      null,
      "This build could not be loaded.",
    ],
    [
      "build 404 with a request",
      null,
      true,
      notFound,
      "Only the person who asked can watch this build.",
    ],
    ["build 500 with a request", null, true, serverError, undefined],
    [
      "request refetch failed but request is known",
      notFound,
      true,
      null,
      undefined,
    ],
    ["nothing wrong", null, true, null, undefined],
  ])("%s", (_name, requestError, hasRequest, buildError, expected) => {
    expect(buildPageNotice({ requestError, hasRequest, buildError })).toBe(
      expected
    )
  })
})

describe("buildPageTitle", () => {
  it.each([
    ["a built spec wins", "Spec", "Build", "Saved", "Spec"],
    ["then the build title", undefined, "Build", "Saved", "Build"],
    [
      "a build title hidden from the viewer is skipped",
      undefined,
      null,
      "Saved",
      "Saved",
    ],
    [
      "a queued revision uses the dashboard name",
      undefined,
      undefined,
      "Saved",
      "Saved",
    ],
    [
      "a new dashboard before its first save has none",
      undefined,
      undefined,
      undefined,
      undefined,
    ],
  ])("%s", (_name, specTitle, buildTitle, dashboardTitle, expected) => {
    expect(buildPageTitle({ specTitle, buildTitle, dashboardTitle })).toBe(
      expected
    )
  })
})
