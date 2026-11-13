import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"
import { renderWithProviders } from "@/test/render"

import { WebhookForm } from "./webhook-form"

function renderForm(serverError: unknown) {
  return renderWithProviders(
    <WebhookForm
      formId="add-webhook"
      serverError={serverError}
      onSubmit={() => undefined}
      firstInputRef={{ current: null }}
    />
  )
}

describe("WebhookForm server error", () => {
  it("shows the error above the fields and focuses it so a long event list cannot hide it", () => {
    renderForm(new ApiError(409, "too_many_webhooks", "Too many."))
    const alert = screen.getByRole("alert")
    expect(alert).toHaveTextContent(
      "At most 20 webhooks can exist. Delete one first."
    )
    expect(alert).toHaveFocus()
    expect(
      alert.compareDocumentPosition(screen.getByLabelText("Name")) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      alert.compareDocumentPosition(
        screen.getByRole("group", { name: "Events to send" })
      ) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it("shows no alert without a server error", () => {
    renderForm(null)
    expect(screen.queryByRole("alert")).toBeNull()
  })
})
