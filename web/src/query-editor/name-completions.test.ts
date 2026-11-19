import { CompletionContext } from "@codemirror/autocomplete"
import { EditorState } from "@codemirror/state"
import { expect, it } from "vitest"

import { nameCompletions } from "./extensions"

function optionsAt(doc: string, explicit: boolean) {
  const state = EditorState.create({
    doc,
    extensions: [nameCompletions(["ServiceName", "SeverityText", "Body"])],
  })
  const [source] = state.languageDataAt<
    (context: CompletionContext) => {
      from: number
      options: { label: string }[]
    } | null
  >("autocomplete", doc.length)
  const result = source(new CompletionContext(state, doc.length, explicit))
  return result === null
    ? null
    : { from: result.from, labels: result.options.map((item) => item.label) }
}

it("offers the names after a word the person is typing", () => {
  expect(optionsAt("SELECT Se", false)).toEqual({
    from: 7,
    labels: ["ServiceName", "SeverityText", "Body"],
  })
})

it("offers the names on Ctrl+Space with nothing typed", () => {
  expect(optionsAt("SELECT ", true)).toEqual({
    from: 7,
    labels: ["ServiceName", "SeverityText", "Body"],
  })
})

it("stays quiet when nothing is typed and completion was not asked for", () => {
  expect(optionsAt("SELECT ", false)).toBeNull()
})
