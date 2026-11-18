import { describe, expect, it } from "vitest"

import { proposalStateText } from "./proposal-state"

describe("proposalStateText", () => {
  it.each([
    [undefined, "Proposal"],
    [
      { state: "open", merged_version: null },
      "Proposal opened, waiting for an Editor",
    ],
    [{ state: "merged", merged_version: 8 }, "Proposal merged as version 8"],
    [{ state: "merged", merged_version: null }, "Proposal merged"],
    [{ state: "discarded", merged_version: null }, "Proposal discarded"],
  ] as const)("reads %#", (proposal, text) => {
    expect(
      proposalStateText(proposal as Parameters<typeof proposalStateText>[0])
    ).toBe(text)
  })
})
