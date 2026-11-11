import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"

import { Composer } from "./composer"

const adaId = "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f"
const people = [{ id: adaId, name: "Ada" }]

function setup(
  props: Partial<Parameters<typeof Composer>[0]> = {},
  onSubmit: (body: string) => Promise<unknown> = () => Promise.resolve()
) {
  const user = userEvent.setup()
  const submit = vi.fn(onSubmit)
  render(
    <Composer
      label="Reply"
      placeholder="Reply, or type @ to ask an agent"
      people={people}
      onSubmit={submit}
      {...props}
    />
  )
  const field = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Reply",
  })
  return { user, submit, field }
}

describe("Composer", () => {
  it("sends the trimmed text on Mod+Enter and clears it once the send resolves", async () => {
    const { user, submit, field } = setup()
    await user.type(field, "  Looks like the deploy  ")
    await user.keyboard("{Control>}{Enter}{/Control}")
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith("Looks like the deploy")
    )
    await waitFor(() => expect(field).toHaveValue(""))
  })

  it("inserts a newline on Enter alone", async () => {
    const { user, submit, field } = setup()
    await user.type(field, "one{Enter}two")
    expect(field).toHaveValue("one\ntwo")
    expect(submit).not.toHaveBeenCalled()
  })

  it("sends from the Send button and keeps it disabled while the text is empty", async () => {
    const { user, submit, field } = setup()
    const send = screen.getByRole("button", { name: "Send" })
    expect(send).toBeDisabled()
    await user.type(field, "Agreed")
    await user.click(send)
    await waitFor(() => expect(submit).toHaveBeenCalledWith("Agreed"))
  })

  it("shows the server's sentence for a rejected send and keeps the text", async () => {
    const { user, field } = setup({}, () =>
      Promise.reject(new ApiError(400, "invalid", "comment is too long"))
    )
    await user.type(field, "Agreed")
    await user.click(screen.getByRole("button", { name: "Send" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Comment is too long."
    )
    expect(field).toHaveValue("Agreed")
  })

  it("shows a generic line when the server fails", async () => {
    const { user } = setup({}, () =>
      Promise.reject(new ApiError(500, "internal", "boom"))
    )
    await user.type(screen.getByRole("textbox", { name: "Reply" }), "Agreed")
    await user.click(screen.getByRole("button", { name: "Send" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The comment could not be sent. Try again."
    )
  })

  it("calls onCancel on Escape", async () => {
    const onCancel = vi.fn()
    const { user, field } = setup({ onCancel })
    await user.click(field)
    await user.keyboard("{Escape}")
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it("opens the mention picker on a typed @ and leaves one token when an agent is chosen", async () => {
    const { user, field } = setup()
    await user.type(field, "@")
    await user.click(
      await screen.findByRole("option", { name: /Observe Agent/ })
    )
    await waitFor(() => expect(field).toHaveValue("@observe "))
  })

  it("replaces the typed @ with a person's token after text", async () => {
    const { user, field } = setup()
    await user.type(field, "ask @")
    await user.click(await screen.findByRole("option", { name: "Ada" }))
    await waitFor(() => expect(field).toHaveValue(`ask @[Ada](user:${adaId}) `))
  })

  it("does not open the picker for an @ inside a word", async () => {
    const { user, field } = setup()
    await user.type(field, "mail@")
    expect(screen.queryByRole("option")).toBeNull()
  })

  it("keeps the @, closes the picker and refocuses the field on Escape, without cancelling", async () => {
    const onCancel = vi.fn()
    const { user, field } = setup({ onCancel })
    await user.type(field, "@")
    await screen.findAllByRole("option")
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("option")).toBeNull())
    expect(field).toHaveValue("@")
    await waitFor(() => expect(field).toHaveFocus())
    expect(onCancel).not.toHaveBeenCalled()
  })

  it("shows why the reader cannot write and disables sending", () => {
    const { field } = setup({
      hint: "Stays here until added to the page",
      disabledReason: "Only people who can comment write here",
    })
    expect(
      screen.getByText("Only people who can comment write here")
    ).toBeInTheDocument()
    expect(screen.queryByText("Stays here until added to the page")).toBeNull()
    expect(field).toBeDisabled()
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled()
  })

  it("shows the hint when the reader can write", () => {
    setup({ hint: "Stays here until added to the page" })
    expect(
      screen.getByText("Stays here until added to the page")
    ).toBeInTheDocument()
  })
})
