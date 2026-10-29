import { describe, expect, it } from "vitest"

import { compileNamePattern } from "./patterns"

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
