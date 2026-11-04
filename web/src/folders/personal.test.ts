import { describe, expect, it } from "vitest"

import { isOrphanedPersonal, mayMoveInto } from "./personal"
import type { Folder } from "./queries"

const me = { kind: "user", id: "u-me", name: "Me" } as const
const other = { kind: "user", id: "u-other", name: "Other" } as const

function folder(id: string, extra: Partial<Folder> = {}): Folder {
  return {
    id,
    title: id,
    general: false,
    personal: false,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: false, create: true },
    level: "edit",
    ...extra,
  }
}

const mine = folder("mine", { personal: true, created_by: me })
const orphan = folder("orphan", { personal: true, created_by: other })
const shared = folder("shared", { created_by: me })

describe("isOrphanedPersonal", () => {
  it("is false for the principal's own personal folder", () => {
    expect(isOrphanedPersonal(mine, "u-me")).toBe(false)
  })

  it("is true for a personal folder owned by someone else", () => {
    expect(isOrphanedPersonal(orphan, "u-me")).toBe(true)
  })

  it("is false for a shared folder", () => {
    expect(isOrphanedPersonal(shared, "u-other")).toBe(false)
  })
})

describe("mayMoveInto", () => {
  it.each([
    ["any creator into a shared folder", shared, other, true],
    ["the principal's own dashboard into My Dashboards", mine, me, true],
    ["someone else's dashboard into My Dashboards", mine, other, false],
    [
      "a service account with the principal's id into My Dashboards",
      mine,
      { kind: "service_account", id: "u-me" },
      false,
    ],
  ] as const)("%s", (_name, target, creator, expected) => {
    expect(mayMoveInto(target, creator, "u-me")).toBe(expected)
  })
})
