import { describe, expect, test } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import {
  contextProblemText,
  createBody,
  failureText,
  hasReadOnlyToApprove,
  isMarkedReadOnly,
  sourceProblems,
  sourceStatus,
  toolStatus,
  toolSummary,
  updateBody,
  type SourceInput,
} from "./problems"
import type { MCPSource, MCPTool } from "./queries"

const input: SourceInput = {
  uid: "docs",
  name: "Docs",
  description: "",
  url: "https://mcp.example.com/mcp",
  audience: "everyone",
  secretHeader: "",
  secret: "",
  clearSecret: false,
}

const counts = { offered: 0, approved: 0, changed: 0, gone: 0, refused: 0 }

function source(overrides: Partial<MCPSource> = {}): MCPSource {
  return {
    uid: "docs",
    name: "Docs",
    description: null,
    url: "https://mcp.example.com/mcp",
    audience: "everyone",
    secret_header: null,
    secret_set: false,
    provisioned: false,
    private: false,
    state: "unchecked",
    failure: null,
    checked_at: null,
    tools: counts,
    created_at: "2026-11-25T10:00:00Z",
    updated_at: "2026-11-25T10:00:00Z",
    ...overrides,
  }
}

function tool(overrides: Partial<MCPTool> = {}): MCPTool {
  return {
    name: "search",
    state: "offered",
    refusal: null,
    description: "Search",
    input_schema: {},
    read_only: true,
    hash: "a".repeat(64),
    seen: null,
    approved_by: null,
    approved_at: null,
    updated_at: "2026-11-25T10:00:00Z",
    allowed_actions: [],
    ...overrides,
  }
}

describe("sourceProblems", () => {
  test("accepts a plain registration", () => {
    expect(sourceProblems(input, "add", false)).toEqual({})
  })

  test("asks for a usable id only when adding", () => {
    expect(
      sourceProblems({ ...input, uid: "bad id" }, "add", false).uid
    ).toBeDefined()
    expect(
      sourceProblems({ ...input, uid: "" }, "add", false).uid
    ).toBeDefined()
    expect(
      sourceProblems({ ...input, uid: "" }, "edit", false).uid
    ).toBeUndefined()
  })

  test.each([
    "",
    "mcp.example.com/mcp",
    "http://mcp.example.com/mcp",
    "https://localhost/mcp",
    "https://127.0.0.1/mcp",
    "https://[::1]/mcp",
    "https://user:pass@mcp.example.com/mcp",
    "https://mcp.example.com/mcp?key=abc",
    "https://mcp.example.com/mcp#frag",
    "https://mcp.example.com/mcp?",
  ])("refuses the address %j", (url) => {
    expect(sourceProblems({ ...input, url }, "add", false).url).toBeDefined()
  })

  test("needs the header whenever a secret is sent or kept", () => {
    expect(
      sourceProblems({ ...input, secret: "s3cret" }, "add", false).secretHeader
    ).toBeDefined()
    expect(
      sourceProblems(
        { ...input, secret: "s3cret", secretHeader: "X-Api-Key" },
        "add",
        false
      )
    ).toEqual({})
    expect(sourceProblems(input, "edit", true).secretHeader).toBeDefined()
    expect(
      sourceProblems({ ...input, clearSecret: true }, "edit", true)
    ).toEqual({})
  })

  test("limits the description to one line of at most 300 characters", () => {
    const refused = (description: string) =>
      sourceProblems({ ...input, description }, "add", false).description
    expect(refused("Runbooks and design notes")).toBeUndefined()
    expect(refused("é".repeat(300))).toBeUndefined()
    expect(refused("é".repeat(301))).toBeDefined()
    expect(refused("two\nlines")).toBeDefined()
    expect(refused("zero\u200Bwidth")).toBeDefined()
    expect(refused("next\u0085line")).toBeDefined()
  })

  test("refuses a name with a control or invisible character", () => {
    const refused = (name: string) =>
      sourceProblems({ ...input, name }, "add", false).name
    expect(refused("Company docs")).toBeUndefined()
    expect(refused("Co\u200Dmpany")).toBeDefined()
    expect(refused("Company\u0085docs")).toBeDefined()
    expect(refused("Company\ndocs")).toBeDefined()
  })

  test("refuses a secret with a line break", () => {
    const problems = sourceProblems(
      { ...input, secret: "a\nb", secretHeader: "X-Api-Key" },
      "add",
      false
    )
    expect(problems.secret).toBeDefined()
  })
})

describe("request bodies", () => {
  test("a registration sends the secret and its header together, and no empty name", () => {
    expect(createBody({ ...input, name: " " })).toEqual({
      uid: "docs",
      url: "https://mcp.example.com/mcp",
      audience: "everyone",
    })
    expect(
      createBody({ ...input, secret: "s3cret", secretHeader: " X-Api-Key " })
    ).toMatchObject({ secret: "s3cret", secret_header: "X-Api-Key" })
  })

  test("a description is sent when set, and an edit can remove it", () => {
    expect(createBody({ ...input, description: " Runbooks " })).toMatchObject({
      description: "Runbooks",
    })
    expect(createBody(input)).not.toHaveProperty("description")
    const described = source({ description: "Runbooks" })
    expect(
      updateBody(described, { ...input, description: "Runbooks" })
    ).toEqual({})
    expect(updateBody(described, { ...input, description: "Wiki" })).toEqual({
      description: "Wiki",
    })
    expect(updateBody(described, { ...input, description: " " })).toEqual({
      description: "",
    })
    expect(updateBody(source(), input)).toEqual({})
  })

  test("an edit sends only what changed", () => {
    expect(updateBody(source(), input)).toEqual({})
    expect(
      updateBody(source(), { ...input, name: "Renamed", audience: "admins" })
    ).toEqual({ name: "Renamed", audience: "admins" })
    expect(
      updateBody(source(), { ...input, url: "https://other.example.com/mcp" })
    ).toEqual({ url: "https://other.example.com/mcp" })
  })

  test("an edit that clears the secret sends nothing else about it", () => {
    expect(
      updateBody(source({ secret_set: true, secret_header: "X-Api-Key" }), {
        ...input,
        secretHeader: "X-Api-Key",
        secret: "ignored",
        clearSecret: true,
      })
    ).toEqual({ clear_secret: true })
  })

  test("an edit that replaces the secret sends it with the header", () => {
    expect(
      updateBody(source({ secret_set: true, secret_header: "X-Api-Key" }), {
        ...input,
        secretHeader: "X-Api-Key",
        secret: "new",
      })
    ).toEqual({ secret: "new" })
  })
})

describe("contextProblemText", () => {
  test("names the key to set when there is none", () => {
    expect(
      contextProblemText(new ApiError(409, "secret_key_missing", "x"))
    ).toContain("SLASHOBSERVE_SECRET_KEY")
  })

  test("explains a changed tool and a refused tool", () => {
    expect(
      contextProblemText(new ApiError(409, "tool_changed", "x"))
    ).toContain("differently")
    expect(
      contextProblemText(new ApiError(409, "tool_refused", "x"))
    ).toContain("destructive")
  })

  test("falls back to the server's message, and to a network sentence", () => {
    expect(
      contextProblemText(new ApiError(422, "invalid_mcp_source", "url: bad"))
    ).toBe("url: bad")
    expect(contextProblemText(new NetworkError("down"))).not.toBe("")
  })
})

describe("status text", () => {
  test("a source shows its state, a failure reason and its tool counts", () => {
    expect(sourceStatus(source()).tone).toBe("none")
    expect(
      sourceStatus(source({ state: "unreachable", failure: "unauthorized" }))
    ).toMatchObject({
      tone: "critical",
      detail: "The server refused the secret",
    })
    expect(
      sourceStatus(
        source({
          state: "ready",
          tools: { ...counts, approved: 2, changed: 1 },
        })
      )
    ).toEqual({ tone: "ok", label: "Ready", detail: "2 approved, 1 changed" })
  })

  test("an unknown failure code shows as it is", () => {
    expect(failureText("odd")).toBe("odd")
    expect(failureText(null)).toBe("The last check failed")
  })

  test("toolSummary leaves out empty counts", () => {
    expect(toolSummary(counts)).toBe("0 approved")
    expect(toolSummary({ ...counts, offered: 3, refused: 1 })).toBe(
      "0 approved, 3 to review, 1 refused"
    )
  })

  test("a changed tool says it is not served, and a refused one says why", () => {
    expect(toolStatus(tool({ state: "changed" })).detail).toContain(
      "not served"
    )
    expect(
      toolStatus(
        tool({
          state: "refused",
          refusal: "The server marks this tool as destructive.",
        })
      )
    ).toMatchObject({
      tone: "critical",
      detail: "The server marks this tool as destructive.",
    })
  })

  test("only an offered tool can be approved in bulk", () => {
    expect(hasReadOnlyToApprove([tool({ state: "approved" })])).toBe(false)
    expect(hasReadOnlyToApprove([tool({ state: "refused" }), tool()])).toBe(
      true
    )
    expect(hasReadOnlyToApprove([tool({ read_only: false })])).toBe(false)
  })

  test("a changed tool is marked read-only as the server describes it now", () => {
    expect(isMarkedReadOnly(tool({ read_only: false }))).toBe(false)
    expect(
      isMarkedReadOnly(
        tool({
          state: "changed",
          seen: {
            description: "Search",
            input_schema: {},
            read_only: false,
            hash: "b".repeat(64),
          },
        })
      )
    ).toBe(false)
  })
})
