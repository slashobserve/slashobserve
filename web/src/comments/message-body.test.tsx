import { render } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { MessageBody } from "./message-body"

const markdownRenders = vi.hoisted(() => vi.fn())

vi.mock("./comment-markdown", async (importOriginal) => {
  const original = await importOriginal<typeof import("./comment-markdown")>()
  return {
    ...original,
    CommentMarkdown: ({ markdown }: { markdown: string }) => {
      markdownRenders()
      return <p>{markdown}</p>
    },
  }
})

describe("MessageBody", () => {
  it("does not parse the Markdown again when its props are unchanged", () => {
    markdownRenders.mockClear()
    const mentions: never[] = []
    const { rerender, getByText } = render(
      <MessageBody body="hello **there**" mentions={mentions} markdown />
    )
    rerender(<MessageBody body="hello **there**" mentions={mentions} markdown />)
    expect(markdownRenders).toHaveBeenCalledTimes(1)
    rerender(<MessageBody body="changed" mentions={mentions} markdown />)
    expect(markdownRenders).toHaveBeenCalledTimes(2)
    expect(getByText("changed")).toBeTruthy()
  })
})
