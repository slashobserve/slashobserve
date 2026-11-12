import { useRef, useState } from "react"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { MentionButton } from "./mention-button"
import { mentionOptions } from "./mentions"

const adaId = "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f"
const people = [
  { id: adaId, name: "Ada" },
  { id: "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d", name: "Grace" },
]

function Harness({
  initial,
  replaceFrom = null,
  initiallyOpen = false,
}: {
  initial: string
  replaceFrom?: number | null
  initiallyOpen?: boolean
}) {
  const [value, setValue] = useState(initial)
  const [open, setOpen] = useState(initiallyOpen)
  const field = useRef<HTMLTextAreaElement>(null)
  return (
    <>
      <textarea
        ref={field}
        aria-label="Body"
        value={value}
        onChange={(event) => setValue(event.target.value)}
      />
      <MentionButton
        options={mentionOptions(people, [])}
        field={field}
        value={value}
        onChange={setValue}
        open={open}
        onOpenChange={setOpen}
        replaceFrom={replaceFrom}
      />
    </>
  )
}

async function setup(initial: string, caret: number) {
  const user = userEvent.setup()
  render(<Harness initial={initial} />)
  const body = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Body",
  })
  body.focus()
  body.setSelectionRange(caret, caret)
  await user.click(
    screen.getByRole("combobox", { name: "Mention a person or an agent" })
  )
  return { user, body }
}

describe("MentionButton", () => {
  it("offers @observe and the people it is given", async () => {
    await setup("", 0)
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "Observe Agent@observe",
      "Ada",
      "Grace",
    ])
  })

  it("inserts a user token at the caret", async () => {
    const { user, body } = await setup("ping now", 5)
    await user.click(await screen.findByRole("option", { name: "Ada" }))
    await waitFor(() =>
      expect(body).toHaveValue(`ping @[Ada](user:${adaId}) now`)
    )
  })

  it("inserts the agent handle", async () => {
    const { user, body } = await setup("", 0)
    await user.click(
      await screen.findByRole("option", { name: /Observe Agent/ })
    )
    await waitFor(() => expect(body).toHaveValue("@observe "))
  })

  it("filters the people by what is typed", async () => {
    const { user } = await setup("", 0)
    await user.keyboard("gra")
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual(["Grace"])
  })

  it("replaces the typed @ when it was opened by typing", async () => {
    const user = userEvent.setup()
    render(<Harness initial="hi @" replaceFrom={3} initiallyOpen />)
    const body = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Body",
    })
    await user.click(await screen.findByRole("option", { name: /Observe/ }))
    await waitFor(() => expect(body).toHaveValue("hi @observe "))
  })

  it("closes on Escape, keeps the text and returns focus to the field", async () => {
    const { user, body } = await setup("hi @", 4)
    await screen.findAllByRole("option")
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("option")).toBeNull())
    expect(body).toHaveValue("hi @")
    await waitFor(() => expect(body).toHaveFocus())
  })
})
