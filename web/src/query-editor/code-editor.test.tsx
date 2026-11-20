import { act, render, waitFor } from "@testing-library/react"
import { createRef } from "react"
import { describe, expect, it } from "vitest"

import { CodeEditor, type CodeEditorHandle } from "./code-editor"

async function openWithCompletion(focused = true) {
  const handle = createRef<CodeEditorHandle>()
  const view = render(
    <CodeEditor
      value="che"
      onChange={() => {}}
      datasource={{ uid: "none" }}
      readOnly={false}
      ariaLabel="Query"
      onLint={() => {}}
      editorRef={handle}
      completionNames={["checkout"]}
      autoFocus={focused}
    />
  )
  const content = view.container.querySelector<HTMLElement>(".cm-content")
  if (content === null) throw new Error("editor did not mount")
  if (focused) content.focus()
  act(() => {
    content.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: " ",
        code: "Space",
        ctrlKey: true,
        bubbles: true,
      })
    )
  })
  return handle
}

describe("CodeEditor completions", () => {
  it("closes the popup when asked to", async () => {
    const handle = await openWithCompletion()
    await waitFor(() =>
      expect(document.querySelector(".cm-tooltip-autocomplete")).not.toBeNull()
    )
    act(() => handle.current?.closeCompletions())
    await waitFor(() =>
      expect(document.querySelector(".cm-tooltip-autocomplete")).toBeNull()
    )
  })

  it("does not leave a popup open that a late result opened in an unfocused editor", async () => {
    await openWithCompletion(false)
    await new Promise((resolve) => setTimeout(resolve, 100))
    expect(document.querySelector(".cm-tooltip-autocomplete")).toBeNull()
  })
})
