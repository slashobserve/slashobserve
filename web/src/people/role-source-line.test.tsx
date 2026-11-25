import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { RoleSourceLine } from "./role-source-line"

describe("RoleSourceLine", () => {
  it("keeps the source on one line and shows it whole on hover", () => {
    render(
      <RoleSourceLine
        person={{
          role_source: "admin_emails",
          role_group: null,
          role_set_by: null,
        }}
      />
    )

    const line = screen.getByTitle("From SLASHOBSERVE_ADMIN_EMAILS")
    expect(line).toHaveClass("truncate")
    expect(line).toHaveTextContent("From SLASHOBSERVE_ADMIN_EMAILS")
  })
})
