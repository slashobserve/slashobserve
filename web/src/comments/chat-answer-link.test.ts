import { describe, expect, it } from "vitest"

import { chatAnswerLink } from "./chat-answer-link"

const run = "33333333-3333-4333-8333-333333333333"
const other = "44444444-4444-4444-8444-444444444444"
const reply = `Answered in the chat.\n\nthe 5xx rate rose\n\nFull answer (only the person who asked and Admins can open it): [the chat](/c/${run})`

describe("chatAnswerLink", () => {
  it("splits the closing link to the run's own chat from the text before it", () => {
    expect(chatAnswerLink(reply, run)).toEqual({
      before: `Answered in the chat.\n\nthe 5xx rate rose\n\nFull answer (only the person who asked and Admins can open it): `,
      label: "the chat",
      runId: run,
    })
  })

  it.each([
    ["a reply with no link", "Saved version 2 of this dashboard.", run],
    ["a link to another chat", reply, other],
    ["a reply with no run", reply, null],
    ["a link that is not last", `${reply} thanks`, run],
    ["an outside link", "[x](https://example.com/c/" + run + ")", run],
    ["a script link", "[x](javascript:alert(1))", run],
    ["a protocol-relative link", `[x](//evil.example/c/${run})`, run],
  ])("finds nothing in %s", (_name, body, runId) => {
    expect(chatAnswerLink(body, runId)).toBeNull()
  })
})
