import { describe, expect, it } from "vitest"

import {
  chatCommands,
  matchingCommands,
  takeCommand,
  withCommand,
  type SlashCommand,
} from "./slash-commands"

const all: SlashCommand[] = ["investigate", "import"]

describe("takeCommand", () => {
  it("takes an available command and the text after it", () => {
    expect(takeCommand("/investigate   why p99", all)).toEqual({
      command: "investigate",
      rest: "why p99",
    })
  })

  it("takes a command followed only by its space", () => {
    expect(takeCommand("/import ", all)).toEqual({
      command: "import",
      rest: "",
    })
  })

  it("keeps the lines after the command", () => {
    expect(takeCommand("/investigate why\nnow", all)).toEqual({
      command: "investigate",
      rest: "why\nnow",
    })
  })

  it.each([
    ["an unknown command", "/foo bar", all],
    ["a command without a space", "/import", all],
    ["a command that is not available", "/investigate why", ["import"]],
    ["a command that no longer exists", "/revise add", all],
    ["text that does not start with a slash", "import add", all],
  ] as const)("leaves %s as plain text", (_name, text, available) => {
    expect(takeCommand(text, available)).toBeNull()
  })
})

describe("matchingCommands", () => {
  it("lists the available commands a typed word starts", () => {
    expect(matchingCommands("/i", all)).toEqual(["investigate", "import"])
    expect(matchingCommands("/", ["import"])).toEqual(["import"])
  })

  it.each(["", "/import x", "/x", "/revise", "hello", "/Imp"])(
    "lists nothing for %j",
    (text) => {
      expect(matchingCommands(text, all)).toEqual([])
    }
  )
})

describe("withCommand", () => {
  it("prefixes the command and keeps plain text unchanged", () => {
    expect(withCommand("import", "a link")).toBe("/import a link")
    expect(withCommand(null, "add a row")).toBe("add a row")
  })
})

describe("chatCommands", () => {
  it("offers Investigate only to someone who may investigate", () => {
    expect(chatCommands(true)).toEqual(["investigate", "import"])
    expect(chatCommands(false)).toEqual(["import"])
  })
})
