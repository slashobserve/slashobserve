import {
  CompletionContext,
  currentCompletions,
  startCompletion,
} from "@codemirror/autocomplete"
import { EditorState } from "@codemirror/state"
import { EditorView } from "@codemirror/view"
import { expect, it, vi } from "vitest"

import { languageFor, nameCompletions } from "./extensions"
import { must } from "@/test/must"

function optionsAt(doc: string, explicit: boolean) {
  const state = EditorState.create({
    doc,
    extensions: [nameCompletions(["ServiceName", "SeverityText", "Body"])],
  })
  const source = state.languageDataAt<
    (context: CompletionContext) => {
      from: number
      options: { label: string }[]
    } | null
  >("autocomplete", doc.length)[0]
  const result = must(source)(
    new CompletionContext(state, doc.length, explicit)
  )
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

async function offeredInEditor(type: string, doc: string) {
  const view = new EditorView({
    parent: document.body,
    state: EditorState.create({
      doc,
      selection: { anchor: doc.length },
      extensions: [
        languageFor(type),
        nameCompletions(["ServiceName", "SeverityText", "Body"]),
      ],
    }),
  })
  startCompletion(view)
  await vi.waitFor(() =>
    expect(currentCompletions(view.state).length).toBeGreaterThan(0)
  )
  const labels = currentCompletions(view.state).map((item) => item.label)
  view.destroy()
  return labels
}

it("keeps offering the names in a running editor on ClickHouse next to the SQL keywords", async () => {
  const labels = await offeredInEditor(
    "clickhouse",
    "SELECT Body FROM otel_logs WHERE Serv"
  )
  expect(labels).toContain("ServiceName")
  expect(labels).toContain("PRESERVE")
})

it("offers the names in a running editor on Loki", async () => {
  expect(await offeredInEditor("loki", "{Serv")).toEqual(["ServiceName"])
})
