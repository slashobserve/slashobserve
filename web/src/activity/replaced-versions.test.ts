import { describe, expect, it } from "vitest"

import { replacedVersions } from "./replaced-versions"

function saved(data: Record<string, unknown>) {
  return { data: data as Record<string, never> }
}

describe("replacedVersions", () => {
  it("maps a version to the version its file replaced", () => {
    const replaced = replacedVersions([
      saved({ version: 5, replaced_version: 4 }),
      saved({ version: 3, parent_version: 2 }),
      saved({ version: 2, replaced_version: 1 }),
    ])
    expect([...replaced]).toEqual([
      [5, 4],
      [2, 1],
    ])
  })

  it("leaves out an event whose numbers are not version numbers", () => {
    const replaced = replacedVersions([
      saved({ version: 5, replaced_version: 0 }),
      saved({ version: "5", replaced_version: 4 }),
      saved({ replaced_version: 4 }),
      saved({ version: 6, replaced_version: 4.5 }),
    ])
    expect(replaced.size).toBe(0)
  })
})
