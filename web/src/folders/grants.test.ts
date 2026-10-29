import { describe, expect, test } from "vitest"

import { entriesForPut, levelOptions, parseGrantee, type Entry } from "./grants"

const priya: Entry = {
  kind: "user",
  user: {
    id: "11111111-1111-4111-8111-111111111111",
    name: "Priya",
    email: "priya@example.com",
    role: "viewer",
  },
  level: "view",
}

const ishan: Entry = {
  kind: "user",
  user: {
    id: "22222222-2222-4222-8222-222222222222",
    name: "Ishan",
    email: "ishan@example.com",
    role: "admin",
  },
  level: "admin",
}

const sre: Entry = {
  kind: "group",
  group: "sre",
  member_count: 6,
  level: "edit",
}

describe("parseGrantee", () => {
  test.each([
    ["a person's email", "priya@example.com", { email: "priya@example.com" }],
    [
      "an email with capitals",
      "Priya@Example.COM",
      { email: "priya@example.com" },
    ],
    [
      "an email with spaces around it",
      "  priya@example.com  ",
      { email: "priya@example.com" },
    ],
    ["a group name", "sre", { group: "sre" }],
    ["a group name with capitals kept", "  SRE Team ", { group: "SRE Team" }],
    [
      "a group of exactly 256 bytes",
      "g".repeat(256),
      { group: "g".repeat(256) },
    ],
  ])("accepts %s", (_name, text, expected) => {
    expect(parseGrantee(text)).toEqual(expected)
  })

  test.each([
    ["nothing", "", "Enter an email or a group name."],
    ["only spaces", "   ", "Enter an email or a group name."],
    [
      "an at sign at the start",
      "@example.com",
      "Enter a full email address, like name@example.com.",
    ],
    [
      "an at sign at the end",
      "priya@",
      "Enter a full email address, like name@example.com.",
    ],
    [
      "a lone at sign",
      "@",
      "Enter a full email address, like name@example.com.",
    ],
    [
      "a group of 257 bytes",
      "g".repeat(257),
      "A group name can be at most 256 bytes.",
    ],
    [
      "a group whose bytes, not characters, pass 256",
      "é".repeat(129),
      "A group name can be at most 256 bytes.",
    ],
    [
      "a group with a control character",
      "sre\tteam",
      "A group name cannot contain control characters.",
    ],
  ])("refuses %s", (_name, text, error) => {
    expect(parseGrantee(text)).toEqual({ error })
  })
})

describe("entriesForPut", () => {
  test("adds a person by email after the listed rows", () => {
    expect(
      entriesForPut([priya, sre], {
        kind: "add",
        grantee: { email: "new@example.com" },
        level: "edit",
      })
    ).toEqual([
      { user_id: priya.user!.id, level: "view" },
      { group: "sre", level: "edit" },
      { email: "new@example.com", level: "edit" },
    ])
  })

  test("adds a group", () => {
    expect(
      entriesForPut([priya], {
        kind: "add",
        grantee: { group: "platform" },
        level: "view",
      })
    ).toEqual([
      { user_id: priya.user!.id, level: "view" },
      { group: "platform", level: "view" },
    ])
  })

  test("adding someone already listed changes their level instead of repeating them", () => {
    expect(
      entriesForPut([priya, sre], {
        kind: "add",
        grantee: { email: "priya@example.com" },
        level: "edit",
      })
    ).toEqual([
      { user_id: priya.user!.id, level: "edit" },
      { group: "sre", level: "edit" },
    ])
    expect(
      entriesForPut([priya, sre], {
        kind: "add",
        grantee: { group: "sre" },
        level: "view",
      })
    ).toEqual([
      { user_id: priya.user!.id, level: "view" },
      { group: "sre", level: "view" },
    ])
  })

  test("changes one row's level and keeps the others as they are", () => {
    expect(
      entriesForPut([ishan, priya, sre], {
        kind: "setLevel",
        entry: priya,
        level: "edit",
      })
    ).toEqual([
      { user_id: ishan.user!.id, level: "admin" },
      { user_id: priya.user!.id, level: "edit" },
      { group: "sre", level: "edit" },
    ])
    expect(
      entriesForPut([priya, sre], {
        kind: "setLevel",
        entry: sre,
        level: "admin",
      })
    ).toEqual([
      { user_id: priya.user!.id, level: "view" },
      { group: "sre", level: "admin" },
    ])
  })

  test("removes one row", () => {
    expect(
      entriesForPut([ishan, priya, sre], { kind: "remove", entry: priya })
    ).toEqual([
      { user_id: ishan.user!.id, level: "admin" },
      { group: "sre", level: "edit" },
    ])
    expect(entriesForPut([priya, sre], { kind: "remove", entry: sre })).toEqual(
      [{ user_id: priya.user!.id, level: "view" }]
    )
  })

  test("removing the last row sends an empty list", () => {
    expect(entriesForPut([sre], { kind: "remove", entry: sre })).toEqual([])
  })

  test("clearing sends an empty list", () => {
    expect(entriesForPut([ishan, priya, sre], { kind: "clear" })).toEqual([])
  })

  test("existing people are sent by id and never by email", () => {
    const [sent] = entriesForPut([priya], {
      kind: "setLevel",
      entry: priya,
      level: "edit",
    })
    expect(sent).not.toHaveProperty("email")
    expect(sent).toHaveProperty("user_id", priya.user!.id)
  })
})

describe("levelOptions", () => {
  const all = [
    { level: "view", disabledReason: null },
    { level: "edit", disabledReason: null },
    { level: "admin", disabledReason: null },
  ]
  const noAdmin = [
    { level: "view", disabledReason: null },
    { level: "edit", disabledReason: null },
    { level: "admin", disabledReason: "Needs the Editor role or higher" },
  ]

  test.each([
    ["a viewer", priya, noAdmin],
    [
      "a commenter",
      { ...priya, user: { ...priya.user!, role: "commenter" as const } },
      noAdmin,
    ],
    [
      "an editor",
      { ...priya, user: { ...priya.user!, role: "editor" as const } },
      all,
    ],
    ["an admin", ishan, all],
    ["a group", sre, all],
    ["a person about to be added", { email: "new@example.com" }, all],
    ["a group about to be added", { group: "platform" }, all],
  ])("offers %s the right levels", (_name, target, expected) => {
    expect(levelOptions(target)).toEqual(expected)
  })
})
