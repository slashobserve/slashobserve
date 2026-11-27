import { describe, expect, it } from "vitest"

import { addressProblem } from "./address-problem"

describe("addressProblem", () => {
  const open = { example: "https://example.com" }
  const strict = {
    example: "https://hooks.example.com/in",
    https: true,
    empty: "Enter the address.",
    maxBytes: 20,
    publicHost: "hooks.example.com",
  }

  it("accepts http and https unless https is required", () => {
    expect(addressProblem("http://prom:9090", open)).toBeUndefined()
    expect(addressProblem("ftp://prom", open)).toBe(
      "The address must start with http:// or https://."
    )
    expect(addressProblem("http://a.example.com", strict)).toBe(
      "The address must start with https://."
    )
  })

  it("names the example when the address does not parse", () => {
    expect(addressProblem("prom", open)).toBe(
      "Enter a full http or https URL, such as https://example.com."
    )
    expect(addressProblem("hooks", strict)).toBe(
      "Enter a full https address, such as https://hooks.example.com/in."
    )
  })

  it("checks emptiness and length only when asked", () => {
    expect(addressProblem("", strict)).toBe("Enter the address.")
    expect(addressProblem("https://a.example.com/long", strict)).toBe(
      "The address can be at most 20 characters."
    )
    expect(
      addressProblem("x".repeat(30), { ...open, noun: "URL", maxBytes: 20 })
    ).toBe("The URL can be at most 20 characters.")
  })

  it("runs the form's own check before the public host rule", () => {
    const rules = { ...strict, maxBytes: undefined, check: () => "Own rule." }
    expect(addressProblem("https://localhost", rules)).toBe("Own rule.")
    expect(
      addressProblem("https://localhost", { ...rules, check: undefined })
    ).toBe(
      "The address must use a public domain name, like hooks.example.com. localhost and IP addresses are refused."
    )
  })
})
