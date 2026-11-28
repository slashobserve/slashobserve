import { useRef, useState } from "react"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { MentionButton } from "./mention-button"
import { observeAgent } from "./mentions"
import { useInlineMention } from "./use-inline-mention"
import { useMentionPicks } from "./use-mention-picks"

const adaId = "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f"
const people = [
  { id: adaId, name: "Ada" },
  { id: "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d", name: "Grace" },
]

function Harness({
  initial,
  replaceFrom = null,
  initiallyOpen = false,
  target = null,
}: {
  initial: string
  replaceFrom?: number | null
  initiallyOpen?: boolean
  target?: string | null | undefined
}) {
  const picks = useMentionPicks()
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
      <output aria-label="Sent">{picks.wire(value)}</output>
      <MentionButton
        agents={[observeAgent]}
        people={people}
        target={target}
        onPick={picks.pick}
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

function TypedHarness() {
  const picks = useMentionPicks()
  const mention = useInlineMention()
  const [value, setValue] = useState("")
  const field = useRef<HTMLTextAreaElement>(null)
  return (
    <>
      <textarea
        ref={field}
        aria-label="Body"
        value={value}
        onChange={(event) => {
          mention.typed(value, event.target.value, event.target.selectionStart)
          setValue(event.target.value)
        }}
      />
      <output aria-label="Sent">{picks.wire(value)}</output>
      <MentionButton
        agents={[observeAgent]}
        people={people}
        target={null}
        onPick={picks.pick}
        field={field}
        value={value}
        onChange={setValue}
        open={mention.open}
        onOpenChange={mention.onOpenChange}
        replaceFrom={mention.replaceFrom}
      />
    </>
  )
}

async function typing() {
  const user = userEvent.setup()
  renderWithProviders(<TypedHarness />)
  const body = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Body",
  })
  await user.click(body)
  return { user, body }
}

async function setup(initial: string, caret: number, target?: string) {
  const user = userEvent.setup()
  renderWithProviders(<Harness initial={initial} target={target} />)
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

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("MentionButton", () => {
  it("adds the people the server finds for what is typed, even when only the email matches", async () => {
    const urls: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        urls.push(request.url)
        return new Response(
          JSON.stringify({
            people: [
              {
                id: "11111111-1111-4111-8111-111111111111",
                name: "Linus",
                email: "ada.lab@example.com",
              },
            ],
          }),
          { headers: { "content-type": "application/json" } }
        )
      })
    )
    const { user, body } = await setup("", 0, "dashboard:d1")
    await user.keyboard("ada.lab")
    await waitFor(() =>
      expect(urls.some((url) => url.includes("q=ada.lab"))).toBe(true)
    )
    await user.click(await screen.findByRole("option", { name: /Linus/ }))
    await waitFor(() => expect(body).toHaveValue("@Linus "))
    expect(screen.getByLabelText("Sent")).toHaveTextContent(
      "@[Linus](user:11111111-1111-4111-8111-111111111111)"
    )
    expect(
      urls.some(
        (url) =>
          url.includes("target=dashboard%3Ad1") && url.includes("q=ada.lab")
      )
    ).toBe(true)
  })

  it("keeps two people with one name apart", async () => {
    const { user, body } = await setup("", 0)
    await user.click(await screen.findByRole("option", { name: "Ada" }))
    await waitFor(() => expect(body).toHaveValue("@Ada "))
  })

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
    await waitFor(() => expect(body).toHaveValue("ping @Ada now"))
    expect(screen.getByLabelText("Sent")).toHaveTextContent(
      `ping @[Ada](user:${adaId}) now`
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
    renderWithProviders(
      <Harness initial="hi @" replaceFrom={3} initiallyOpen />
    )
    const body = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Body",
    })
    await user.click(await screen.findByRole("option", { name: /Observe/ }))
    await waitFor(() => expect(body).toHaveValue("hi @observe "))
  })

  it("replaces what was typed after the @ when the field kept the focus", async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <Harness initial="hi @obs" replaceFrom={3} initiallyOpen />
    )
    const body = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Body",
    })
    body.setSelectionRange(7, 7)
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

  it("keeps every typed character in the field when @observe and a sentence are typed straight through", async () => {
    const { user, body } = await typing()
    await user.keyboard("@observe add a stat panel for request rate")
    expect(body).toHaveValue("@observe add a stat panel for request rate")
    expect(screen.queryByRole("option")).toBeNull()
    expect(screen.queryByText("No one found")).toBeNull()
    expect(screen.getByLabelText("Sent")).toHaveTextContent(
      "@observe add a stat panel for request rate"
    )
    expect(body).toHaveFocus()
  })

  it("filters on the typed word while the field keeps the focus", async () => {
    const { user, body } = await typing()
    await user.keyboard("@gra")
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual(["Grace"])
    expect(body).toHaveValue("@gra")
    expect(body).toHaveFocus()
  })

  it("closes at once when nobody matches and keeps the text", async () => {
    const { user, body } = await typing()
    await user.keyboard("@zzz and more")
    expect(body).toHaveValue("@zzz and more")
    expect(screen.queryByRole("option")).toBeNull()
    expect(screen.queryByText("No one found")).toBeNull()
  })

  it("closes at the space after an unknown handle and keeps the text", async () => {
    const { user, body } = await typing()
    await user.keyboard("@gr")
    await screen.findByRole("option", { name: "Grace" })
    await user.keyboard("x add")
    expect(body).toHaveValue("@grx add")
    expect(screen.queryByRole("option")).toBeNull()
  })

  it("does not open for an email address typed mid-sentence", async () => {
    const { user, body } = await typing()
    await user.keyboard("write to a@b.com now")
    expect(body).toHaveValue("write to a@b.com now")
    expect(screen.queryByRole("option")).toBeNull()
  })

  it("picks the highlighted match on Enter and keeps typing after it", async () => {
    const { user, body } = await typing()
    await user.keyboard("hi @gra{Enter}")
    await waitFor(() => expect(body).toHaveValue("hi @Grace "))
    await user.keyboard("now")
    expect(body).toHaveValue("hi @Grace now")
    expect(screen.queryByRole("option")).toBeNull()
  })

  it("closes the list once Enter has picked a mention", async () => {
    const { user, body } = await typing()
    await user.keyboard("@ob{Enter}")
    await waitFor(() => expect(body).toHaveValue("@observe "))
    await waitFor(() => expect(screen.queryByRole("option")).toBeNull())
    expect(body).toHaveFocus()
  })

  it("moves the highlight with the arrow keys before Enter", async () => {
    const { user, body } = await typing()
    await user.keyboard("@")
    await screen.findAllByRole("option")
    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}")
    await waitFor(() => expect(body).toHaveValue("@Ada "))
  })

  it("closes on Escape and keeps the text typed so far", async () => {
    const { user, body } = await typing()
    await user.keyboard("@gra")
    await screen.findAllByRole("option")
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("option")).toBeNull())
    expect(body).toHaveValue("@gra")
    await user.keyboard("c")
    expect(body).toHaveValue("@grac")
  })
})
