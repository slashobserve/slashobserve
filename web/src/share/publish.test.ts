import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import { publishAudience, publishTargets } from "./publish"

function folder(id: string, extra: Partial<Folder> = {}): Folder {
  return {
    id,
    title: id,
    general: false,
    personal: false,
    restricted: false,
    admins_only: false,
    audit: false,
    kind: "open",
    allowed_actions: ["create_in", "move_into"],
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: false, create: true },
    level: "edit",
    ...extra,
  }
}

const viewOnly: Folder["permissions"] = {
  comment: false,
  edit: false,
  manage: false,
  create: false,
}

const ownFolder: Partial<Folder> = {
  personal: true,
  kind: "personal",
  allowed_actions: ["create_in"],
}

function ids(folders: Folder[]): string[] {
  return publishTargets(folders).map((target) => target.id)
}

describe("publishTargets", () => {
  it("leaves out a personal folder", () => {
    expect(ids([folder("mine", ownFolder), folder("ops")])).toEqual(["ops"])
  })

  it("leaves out a folder the caller can only view", () => {
    expect(
      ids([
        folder("ro", { permissions: viewOnly, allowed_actions: [] }),
        folder("ops"),
      ])
    ).toEqual(["ops"])
  })

  it("includes General when the caller can edit it", () => {
    expect(
      ids([
        folder("general", { general: true, kind: "general" }),
        folder("locked", {
          general: true,
          kind: "general",
          permissions: viewOnly,
          allowed_actions: [],
        }),
      ])
    ).toEqual(["general"])
  })

  it("keeps the order it was given", () => {
    expect(ids([folder("b"), folder("a"), folder("c")])).toEqual([
      "b",
      "a",
      "c",
    ])
  })

  it("is empty when nothing qualifies", () => {
    expect(ids([folder("mine", ownFolder)])).toEqual([])
  })
})

describe("publishAudience", () => {
  it("says everyone by role for an open folder", () => {
    expect(publishAudience(folder("ops"))).toMatch(
      /^Everyone on this server can open it/
    )
  })

  it("lists who a restricted folder admits", () => {
    expect(
      publishAudience(
        folder("Payments", {
          title: "Payments",
          restricted: true,
          admins_only: false,
          grantees: ["Ada", "sre"],
          grantee_count: 2,
        })
      )
    ).toBe(
      "Payments is restricted, so only the people and groups on its list (Ada, sre), the person who created it and instance admins can open it."
    )
  })

  it("says only Admins can open an Admins-only folder", () => {
    expect(
      publishAudience(
        folder("Payroll", {
          title: "Payroll",
          restricted: true,
          admins_only: true,
          grantees: [],
          grantee_count: 0,
        })
      )
    ).toBe("Payroll is Admins only, so only instance admins can open it.")
  })

  it("counts the entries it does not name", () => {
    expect(
      publishAudience(
        folder("Payments", {
          title: "Payments",
          restricted: true,
          admins_only: false,
          grantees: ["a", "b", "c", "d", "e"],
          grantee_count: 8,
        })
      )
    ).toContain("(a, b, c, d, e and 3 more)")
  })
})
