import { useRef, useState } from "react"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { MentionButton } from "./mention-button"

const adaId = "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f"
const people = [
  { id: adaId, name: "Ada" },
  { id: "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d", name: "Grace" },
]

function Harness({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial)
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
        people={people}
        field={field}
        value={value}
        onChange={setValue}
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
  await user.click(screen.getByRole("combobox", { name: "Mention" }))
  return { user, body }
}

describe("MentionButton", () => {
  it("offers @observe and the people it is given", async () => {
    await setup("", 0)
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "@observeAgent",
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
    await user.click(await screen.findByRole("option", { name: /@observe/ }))
    await waitFor(() => expect(body).toHaveValue("@observe "))
  })

  it("filters the people by what is typed", async () => {
    const { user } = await setup("", 0)
    await user.keyboard("gra")
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual(["Grace"])
  })
})
