import { describe, expect, it } from "vitest"

import { compileAnchoredPattern, compileNamePattern } from "./patterns"

describe("compileNamePattern", () => {
  it("anchors both ends of every alternative", () => {
    const pattern = compileNamePattern("cpu|mem")
    expect(pattern?.test("cpu")).toBe(true)
    expect(pattern?.test("mem")).toBe(true)
    expect(pattern?.test("cpu_total")).toBe(false)
    expect(pattern?.test("total_mem")).toBe(false)
  })

  it("anchors a plain pattern", () => {
    const pattern = compileNamePattern("http_.*")
    expect(pattern?.test("http_requests")).toBe(true)
    expect(pattern?.test("my_http_requests")).toBe(false)
  })

  it("leaves a slashed pattern unanchored and keeps its flags", () => {
    const pattern = compileNamePattern("/CPU/i")
    expect(pattern?.test("total_cpu")).toBe(true)
  })

  it("matches nothing for an invalid pattern", () => {
    expect(compileNamePattern("(")).toBeNull()
    expect(compileNamePattern("/(/")).toBeNull()
  })
})

describe("compileAnchoredPattern", () => {
  it("anchors every alternative and reads a leading slash as text", () => {
    const pattern = compileAnchoredPattern("/api/.*|health")
    expect(pattern?.test("/api/v1")).toBe(true)
    expect(pattern?.test("health")).toBe(true)
    expect(pattern?.test("a/api/v1")).toBe(false)
    expect(pattern?.test("healthy")).toBe(false)
  })

  it("matches nothing for an invalid pattern", () => {
    expect(compileAnchoredPattern("(")).toBeNull()
  })
})
