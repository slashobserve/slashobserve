import { describe, expect, it } from "vitest"

import { saveToast, type SaveOutcome } from "./save-outcome"

const envelope = {} as Extract<SaveOutcome, { kind: "saved" }>["envelope"]

describe("saveToast", () => {
  it.each<[string, SaveOutcome, string]>([
    [
      "a plain save",
      { kind: "saved", envelope, version: 4, mergedFrom: null },
      "Saved version 4",
    ],
    [
      "a merged save",
      {
        kind: "saved",
        envelope,
        version: 6,
        mergedFrom: { base_version: 3, onto_version: 5 },
      },
      "Saved version 6, merged with changes made since version 3",
    ],
    [
      "a proposal the save opened",
      { kind: "proposed", proposalId: "p1", revision: 1, opened: true },
      "Proposed for an editor to merge",
    ],
    [
      "a revision of an open proposal",
      { kind: "proposed", proposalId: "p1", revision: 3, opened: false },
      "Added to your open proposal (revision 3)",
    ],
  ])("words %s", (_, outcome, text) => {
    expect(saveToast(outcome)).toBe(text)
  })
})
