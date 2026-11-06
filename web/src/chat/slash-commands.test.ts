import { describe, expect, it } from "vitest"

import {
  availableCommands,
  matchingCommands,
  takeCommand,
  withCommand,
  type SlashCommand,
} from "./slash-commands"

const all: SlashCommand[] = ["investigate", "import", "revise"]

describe("takeCommand", () => {
  it("takes an available command and the text after it", () => {
    expect(takeCommand("/revise   add a p99 row", all)).toEqual({
      command: "revise",
      rest: "add a p99 row",
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
    ["a command without a space", "/revise", all],
    ["a command that is not available", "/revise add", ["import"]],
    ["text that does not start with a slash", "revise add", all],
  ] as const)("leaves %s as plain text", (_name, text, available) => {
    expect(takeCommand(text, available)).toBeNull()
  })
})

describe("matchingCommands", () => {
  it("lists the available commands a typed word starts", () => {
    expect(matchingCommands("/i", all)).toEqual(["investigate", "import"])
    expect(matchingCommands("/", ["revise"])).toEqual(["revise"])
  })

  it.each(["", "/revise x", "/x", "hello", "/Rev"])(
    "lists nothing for %j",
    (text) => {
      expect(matchingCommands(text, all)).toEqual([])
    }
  )
})

describe("withCommand", () => {
  it("prefixes the command and keeps plain text unchanged", () => {
    expect(withCommand("revise", "add a row")).toBe("/revise add a row")
    expect(withCommand(null, "add a row")).toBe("add a row")
  })
})

describe("availableCommands", () => {
  it.each([
    [{ hasDashboard: false, surface: "chat" }, ["investigate", "import"]],
    [
      { hasDashboard: true, surface: "chat" },
      ["investigate", "import", "revise"],
    ],
    [{ hasDashboard: true, surface: "activity" }, ["investigate", "revise"]],
  ] as const)("offers the commands for %o", (input, expected) => {
    expect(availableCommands(input)).toEqual(expected)
  })
})
