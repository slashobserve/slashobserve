import { describe, expect, it } from "vitest"

import { confinedNote, type Confined } from "./confined"

describe("confinedNote", () => {
  it("says nothing when nothing confines the observation", () => {
    expect(confinedNote(undefined)).toBeNull()
  })

  it("says an answer keeps the observation with Admins for good", () => {
    expect(confinedNote({ reason: "permanent" })).toMatch(
      /holds an answer that read data only Admins can query/
    )
  })

  it("says a run in flight keeps it there only until the run ends", () => {
    expect(confinedNote({ reason: "run_in_flight" })).toMatch(
      /has not finished\. Until it ends/
    )
  })

  it("says nothing for a reason it does not know", () => {
    expect(confinedNote({ reason: "later" } as unknown as Confined)).toBeNull()
  })
})
