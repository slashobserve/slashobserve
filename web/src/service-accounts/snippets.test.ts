import { describe, expect, test } from "vitest"

import { claudeSnippet, cliSnippet, openCodeSnippet } from "./snippets"

describe("claudeSnippet", () => {
  const snippet = claudeSnippet("https://x/mcp")

  test("adds the server to the project scope with the given URL", () => {
    expect(snippet).toContain("--scope project slashobserve https://x/mcp")
  })

  test("sends the token from the environment in a single-quoted header", () => {
    expect(snippet).toContain("'Authorization: Bearer ${OBSERVE_TOKEN}'")
    expect(snippet).not.toContain('"Authorization')
  })

  test("names every comment tool and the dashboard update tool", () => {
    for (const tool of [
      "list_comments",
      "update_dashboard",
      "reply_to_comment",
      "resolve_comment",
    ]) {
      expect(snippet).toContain(tool)
    }
  })

  test("appends the CLAUDE.md note only when it is not there yet", () => {
    const command = snippet.split("\n").at(-1) as string
    const match =
      /^grep -qF '([^']+)' CLAUDE\.md 2>\/dev\/null \|\| echo '([^']+)' >> CLAUDE\.md$/.exec(
        command
      )
    expect(match).not.toBeNull()
    const [, marker, note] = match as RegExpExecArray
    expect(note).toContain(marker)
  })
})

describe("openCodeSnippet", () => {
  const snippet = openCodeSnippet("https://x/mcp")

  test("turns OAuth off and reads the token from the environment", () => {
    expect(snippet).toContain('"oauth": false')
    expect(snippet).toContain("{env:OBSERVE_TOKEN}")
  })

  test("points at the given URL", () => {
    expect(snippet).toContain('"url": "https://x/mcp"')
  })
})

describe("cliSnippet", () => {
  test("exports the server URL", () => {
    expect(cliSnippet("https://x")).toContain("export OBSERVE_SERVER=https://x")
  })
})

describe("no snippet embeds a key", () => {
  test("contains no sok_ token", () => {
    const snippets = [
      claudeSnippet("https://x/mcp"),
      openCodeSnippet("https://x/mcp"),
      cliSnippet("https://x"),
    ]
    for (const snippet of snippets) {
      expect(snippet).not.toContain("sok_")
    }
  })
})
