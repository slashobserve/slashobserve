import { useEffect, useRef } from "react"
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands"
import { json, jsonParseLinter } from "@codemirror/lang-json"
import { syntaxHighlighting } from "@codemirror/language"
import { linter, lintGutter } from "@codemirror/lint"
import { Compartment, EditorState, Prec } from "@codemirror/state"
import { EditorView, keymap, lineNumbers } from "@codemirror/view"
import { classHighlighter } from "@lezer/highlight"

import { cn } from "@/lib/utils"
import { theme } from "@/query-editor/extensions"
import "@/query-editor/query-editor.css"

type Props = {
  value: string
  onChange: (value: string) => void
  readOnly: boolean
  ariaLabel: string
}

const appearance = Prec.highest(
  EditorView.theme({
    "&": { fontSize: "12px" },
    ".cm-scroller": { lineHeight: "1.6" },
    ".cm-content": { padding: "12px 0" },
    ".cm-gutters": {
      backgroundColor: "transparent",
      borderRight: "1px solid var(--border)",
    },
    ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "transparent" },
  })
)

function accessMode(readOnly: boolean) {
  return [EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]
}

function labelAttributes(ariaLabel: string) {
  return EditorView.contentAttributes.of({ "aria-label": ariaLabel })
}

export function JsonEditor({ value, onChange, readOnly, ariaLabel }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const access = useRef(new Compartment())
  const label = useRef(new Compartment())
  const latest = useRef({ value, onChange, readOnly, ariaLabel })
  useEffect(() => {
    latest.current = { value, onChange, readOnly, ariaLabel }
  })

  useEffect(() => {
    const current = latest.current
    const view = new EditorView({
      parent: host.current ?? undefined,
      state: EditorState.create({
        doc: current.value,
        extensions: [
          json(),
          linter(jsonParseLinter()),
          lintGutter(),
          lineNumbers(),
          history(),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          syntaxHighlighting(classHighlighter),
          EditorView.lineWrapping,
          theme,
          appearance,
          access.current.of(accessMode(current.readOnly)),
          label.current.of(labelAttributes(current.ariaLabel)),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              latest.current.onChange(update.state.doc.toString())
            }
          }),
        ],
      }),
    })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
  }, [])

  useEffect(() => {
    viewRef.current?.dispatch({
      effects: access.current.reconfigure(accessMode(readOnly)),
    })
  }, [readOnly])

  useEffect(() => {
    viewRef.current?.dispatch({
      effects: label.current.reconfigure(labelAttributes(ariaLabel)),
    })
  }, [ariaLabel])

  useEffect(() => {
    const view = viewRef.current
    if (!view || view.state.doc.toString() === value) return
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
    })
  }, [value])

  return (
    <div
      ref={host}
      className={cn(
        "overflow-hidden rounded-control border border-input bg-transparent transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring [&_.cm-editor]:max-h-[60vh] [&_.cm-editor]:min-h-80",
        readOnly && "border-transparent bg-muted/50 dark:bg-muted/30"
      )}
    />
  )
}
