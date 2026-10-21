import { useEffect, useImperativeHandle, useRef, type Ref } from "react"
import { Compartment, EditorState, Prec } from "@codemirror/state"
import { EditorView } from "@codemirror/view"

import { cn } from "@/lib/utils"

import { languageFor } from "./extensions"
import { lintLines } from "./lint-lines"
import { serverLinter, type LintResult } from "./server-lint"
import "./query-editor.css"

export type CodeEditorHandle = {
  focusAt: (from: number) => void
  insertAtCursor: (text: string) => void
}

type Props = {
  value: string
  onChange: (value: string) => void
  datasource: { type?: string; uid: string }
  readOnly: boolean
  ariaLabel: string
  onLint: (result: LintResult) => void
  editorRef?: Ref<CodeEditorHandle>
  autoFocus?: boolean
  onFocus?: () => void
}

const appearance = Prec.highest(
  EditorView.theme({
    "&": { fontSize: "12.5px" },
    ".cm-scroller": { lineHeight: "1.6" },
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

export function CodeEditor({
  value,
  onChange,
  datasource,
  readOnly,
  ariaLabel,
  onLint,
  editorRef,
  autoFocus = false,
  onFocus,
}: Props) {
  const host = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const access = useRef(new Compartment())
  const label = useRef(new Compartment())
  const refocus = useRef(autoFocus)
  const latest = useRef({
    value,
    onChange,
    onLint,
    readOnly,
    ariaLabel,
    onFocus,
  })
  useEffect(() => {
    latest.current = {
      value,
      onChange,
      onLint,
      readOnly,
      ariaLabel,
      onFocus,
    }
  })

  useEffect(() => {
    const current = latest.current
    const view = new EditorView({
      parent: host.current ?? undefined,
      state: EditorState.create({
        doc: current.value,
        extensions: [
          languageFor(datasource.type),
          serverLinter(datasource.uid, (result) =>
            latest.current.onLint(result)
          ),
          lintLines(),
          appearance,
          access.current.of(accessMode(current.readOnly)),
          label.current.of(labelAttributes(current.ariaLabel)),
          EditorView.domEventHandlers({
            focus: () => {
              latest.current.onFocus?.()
            },
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              latest.current.onChange(update.state.doc.toString())
            }
          }),
        ],
      }),
    })
    viewRef.current = view
    if (refocus.current) view.focus()
    return () => {
      refocus.current = view.dom.contains(document.activeElement)
      view.destroy()
      viewRef.current = null
    }
  }, [datasource.type, datasource.uid])

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

  useImperativeHandle(editorRef, () => ({
    focusAt(from) {
      const view = viewRef.current
      if (!view) return
      const anchor = Math.min(Math.max(from, 0), view.state.doc.length)
      view.dispatch({ selection: { anchor }, scrollIntoView: true })
      view.focus()
    },
    insertAtCursor(text) {
      const view = viewRef.current
      if (!view) return
      const range = view.state.selection.main
      view.dispatch({
        changes: { from: range.from, to: range.to, insert: text },
        selection: { anchor: range.from + text.length },
        scrollIntoView: true,
      })
      view.focus()
    },
  }))

  return (
    <div
      ref={host}
      className={cn(
        "overflow-hidden rounded-control border border-input bg-transparent transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring [&_.cm-content]:min-h-14! [&_.cm-editor]:max-h-[50vh]",
        readOnly && "border-transparent bg-muted/50 dark:bg-muted/30"
      )}
    />
  )
}
