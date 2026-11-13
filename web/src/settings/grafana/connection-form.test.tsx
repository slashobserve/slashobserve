import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ConnectionForm } from "./connection-form"

const formId = "add-connection"

function renderForm(serverError: string | null = null) {
  const onSubmit = vi.fn()
  const result = renderWithProviders(
    <>
      <ConnectionForm
        formId={formId}
        serverError={serverError}
        onSubmit={onSubmit}
        firstInputRef={{ current: null }}
      />
      <button type="submit" form={formId}>
        Add connection
      </button>
    </>
  )
  return { ...result, onSubmit }
}

describe("ConnectionForm", () => {
  it("shows an error for each empty field and does not submit", async () => {
    const { user, onSubmit } = renderForm()
    await user.click(
      await screen.findByRole("button", { name: "Add connection" })
    )
    expect(screen.getByLabelText("Name")).toHaveAccessibleDescription(
      "Enter a name."
    )
    expect(screen.getByLabelText("Grafana URL")).toHaveAccessibleDescription(
      "Enter the Grafana URL."
    )
    expect(
      screen.getByLabelText("Service account token")
    ).toHaveAccessibleDescription("Enter a service account token.")
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it("refuses a URL that is not http or https", async () => {
    const { user, onSubmit } = renderForm()
    await user.type(await screen.findByLabelText("Name"), "Production")
    await user.type(screen.getByLabelText("Grafana URL"), "ftp://x")
    await user.type(screen.getByLabelText("Service account token"), "glsa_abc")
    await user.click(screen.getByRole("button", { name: "Add connection" }))
    expect(screen.getByLabelText("Grafana URL")).toHaveAccessibleDescription(
      "The URL must start with http:// or https://."
    )
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it("submits the trimmed values once", async () => {
    const { user, onSubmit } = renderForm()
    await user.type(await screen.findByLabelText("Name"), "  Production ")
    await user.type(
      screen.getByLabelText("Grafana URL"),
      " https://grafana.example.com "
    )
    await user.type(screen.getByLabelText("Service account token"), "glsa_abc ")
    await user.click(screen.getByRole("button", { name: "Add connection" }))
    expect(onSubmit).toHaveBeenCalledExactlyOnceWith({
      name: "Production",
      url: "https://grafana.example.com",
      token: "glsa_abc",
    })
    expect(screen.getByLabelText("Service account token")).toHaveAttribute(
      "type",
      "password"
    )
  })

  it("shows the server's error", async () => {
    renderForm("Set SLASHOBSERVE_SECRET_KEY on the server and restart.")
    expect(
      await screen.findByText(
        "Set SLASHOBSERVE_SECRET_KEY on the server and restart."
      )
    ).toBeInTheDocument()
  })
})
