import { describe, expect, it } from "vitest"

import { lowerFirst, nameParts } from "./text"

describe("lowerFirst", () => {
  it("lowercases the first character only", () => {
    expect(lowerFirst("Yesterday")).toBe("yesterday")
    expect(lowerFirst("In 74 days")).toBe("in 74 days")
    expect(lowerFirst("")).toBe("")
  })
})

describe("nameParts", () => {
  it("offers a break after each dot and underscore", () => {
    expect(nameParts("http.request.method")).toEqual([
      "http.",
      "request.",
      "method",
    ])
    expect(nameParts("http.response.status_code")).toEqual([
      "http.",
      "response.",
      "status_",
      "code",
    ])
  })

  it("offers a break at a camel case boundary", () => {
    expect(nameParts("ResourceAttributes.service.name")).toEqual([
      "Resource",
      "Attributes.",
      "service.",
      "name",
    ])
  })

  it("keeps a plain name whole", () => {
    expect(nameParts("level")).toEqual(["level"])
    expect(nameParts("")).toEqual([""])
    expect(nameParts("HTTP")).toEqual(["HTTP"])
  })
})
