import { describe, expect, test } from "vitest"

import { roleSourceHint } from "./role-source"

describe("roleSourceHint", () => {
  const cases = [
    {
      name: "the admin emails setting",
      person: {
        role_source: "admin_emails",
        role_group: null,
        role_set_by: null,
      },
      hint: "From SLASHOBSERVE_ADMIN_EMAILS",
    },
    {
      name: "the first person to sign in",
      person: {
        role_source: "first_user",
        role_group: null,
        role_set_by: null,
      },
      hint: "First person to sign in",
    },
    {
      name: "an identity provider group",
      person: { role_source: "group", role_group: "sre", role_set_by: null },
      hint: "From the sre group",
    },
    {
      name: "a group whose name is unknown",
      person: { role_source: "group", role_group: null, role_set_by: null },
      hint: "From an identity provider group",
    },
    {
      name: "an Admin's choice",
      person: { role_source: "manual", role_group: null, role_set_by: "Ada" },
      hint: "Set by Ada",
    },
    {
      name: "a choice by an Admin who is no longer known",
      person: { role_source: "manual", role_group: null, role_set_by: null },
      hint: "Set by an Admin",
    },
    {
      name: "the default",
      person: { role_source: "default", role_group: null, role_set_by: null },
      hint: "Default for new people",
    },
  ] as const

  test.each(cases)("explains $name", ({ person, hint }) => {
    expect(roleSourceHint(person)).toBe(hint)
  })
})
