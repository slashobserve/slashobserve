import { describe, expect, it } from "vitest"

import {
  clientText,
  clientTextAtStart,
  labelText,
  returnUrl,
} from "./consent-text"

describe("clientText", () => {
  it("shows the registered name unchanged", () => {
    expect(clientText("Claude Code")).toBe("Claude Code")
    expect(clientTextAtStart("claude code")).toBe("claude code")
  })

  it("names an unnamed client", () => {
    expect(clientText(null)).toBe("an unnamed MCP client")
    expect(clientTextAtStart(null)).toBe("An unnamed MCP client")
  })
})

describe("labelText", () => {
  it("matches the label the server writes", () => {
    expect(labelText("Ada Lovelace", "Claude Code")).toBe(
      "Ada Lovelace (via Claude Code)"
    )
    expect(labelText("Ada Lovelace", null)).toBe("Ada Lovelace (via MCP)")
  })
})

describe("returnUrl", () => {
  it("accepts https and http addresses as given", () => {
    const loopback = "http://localhost:3118/callback?code=c&state=s"
    expect(returnUrl(loopback)).toBe(loopback)
    expect(returnUrl("https://claude.ai/api/mcp/auth_callback?code=c")).toBe(
      "https://claude.ai/api/mcp/auth_callback?code=c"
    )
  })

  it("refuses every other scheme and anything that is not an address", () => {
    expect(returnUrl("javascript:alert(1)")).toBeNull()
    expect(returnUrl("cursor://x/cb?code=c")).toBeNull()
    expect(returnUrl("data:text/html,x")).toBeNull()
    expect(returnUrl("/relative")).toBeNull()
    expect(returnUrl("")).toBeNull()
  })
})
