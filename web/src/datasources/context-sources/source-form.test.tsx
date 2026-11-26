import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import { renderWithProviders } from "@/test/render"

import type { SourceInput } from "./problems"
import type { ContextSource } from "./queries"
import { SourceForm } from "./source-form"

const stored: ContextSource = {
  uid: "docs",
  name: "Docs",
  description: "Runbooks and design notes",
  url: "https://mcp.example.com/mcp",
  audience: "admins",
  secret_header: "X-Api-Key",
  secret_set: true,
  provisioned: false,
  private: false,
  state: "ready",
  failure: null,
  checked_at: null,
  tools: { offered: 0, approved: 0, changed: 0, gone: 0, refused: 0 },
  created_at: "2026-11-25T10:00:00Z",
  updated_at: "2026-11-25T10:00:00Z",
}

function renderForm(
  props: Partial<React.ComponentProps<typeof SourceForm>> = {}
) {
  const onSubmit = vi.fn<(input: SourceInput) => void>()
  const view = renderWithProviders(
    <SourceForm
      formId="source"
      mode="add"
      serverError={null}
      onSubmit={onSubmit}
      firstInputRef={{ current: null }}
      {...props}
    />
  )
  return { ...view, onSubmit }
}

describe("SourceForm", () => {
  it("submits a trimmed registration", async () => {
    const { user, onSubmit } = renderForm()
    await user.type(screen.getByLabelText("Id"), " docs ")
    await user.type(
      screen.getByLabelText("Address"),
      "https://mcp.example.com/mcp"
    )
    await user.type(screen.getByLabelText("Secret header"), "X-Api-Key")
    await user.type(screen.getByLabelText("Secret"), "s3cret")
    document
      .getElementById("source")
      ?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }))
    expect(onSubmit).toHaveBeenCalledWith({
      uid: "docs",
      name: "",
      description: "",
      url: "https://mcp.example.com/mcp",
      audience: "everyone",
      secretHeader: "X-Api-Key",
      secret: "s3cret",
      clearSecret: false,
    })
  })

  it("keeps the secret out of sight and refuses a bad address", async () => {
    const { user, onSubmit } = renderForm()
    expect(screen.getByLabelText("Secret")).toHaveAttribute("type", "password")
    await user.type(screen.getByLabelText("Id"), "docs")
    await user.type(screen.getByLabelText("Address"), "http://localhost/mcp")
    document
      .getElementById("source")
      ?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }))
    expect(onSubmit).not.toHaveBeenCalled()
    expect(await screen.findByText(/must start with https/)).toBeVisible()
  })

  it("offers the audience as a select, not a native one", () => {
    renderForm()
    expect(
      screen.getByRole("combobox", { name: "Who can use its tools" })
    ).toBeVisible()
    expect(document.querySelector("select")).toBeNull()
  })

  it("on edit shows the stored values, never the secret, and can remove it", async () => {
    const { user } = renderForm({ mode: "edit", source: stored })
    expect(screen.queryByLabelText("Id")).toBeNull()
    expect(screen.getByLabelText("Name")).toHaveValue("Docs")
    expect(screen.getByLabelText("Description")).toHaveValue(
      "Runbooks and design notes"
    )
    expect(screen.getByLabelText("Secret")).toHaveValue("")
    expect(screen.getByText(/never shown again/)).toBeVisible()
    await user.click(
      screen.getByRole("checkbox", { name: "Remove the stored secret" })
    )
    expect(screen.getByLabelText("Secret")).toBeDisabled()
    expect(screen.getByLabelText("Secret header")).toBeDisabled()
  })

  it("shows the server's problem above the fields and focuses it", () => {
    renderForm({
      serverError: new ApiError(409, "secret_key_missing", "No key."),
    })
    const alert = screen.getByRole("alert")
    expect(alert).toHaveTextContent("SLASHOBSERVE_SECRET_KEY")
    expect(alert).toHaveFocus()
  })
})
