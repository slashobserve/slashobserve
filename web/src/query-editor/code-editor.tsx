import { useEffect, useImperativeHandle, useRef, type Ref } from "react"
import { Compartment, EditorState } from "@codemirror/state"
import { EditorView } from "@codemirror/view"

import { cn } from "@/lib/utils"

import { languageFor, type QueryLanguage } from "./extensions"
import { serverLinter, type LintResult } from "./server-lint"
import "./query-editor.css"

export type CodeEditorHandle = { focusAt: (from: number) => void }

type Props = {
  value: string
  onChange: (value: string) => void
  datasource: { type: QueryLanguage; uid: string }
  readOnly: boolean
  ariaLabel: string
  onLint: (result: LintResult) => void
  editorRef?: Ref<CodeEditorHandle>
  autoFocus?: boolean
}

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
}: Props) {
  const host = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const access = useRef(new Compartment())
  const label = useRef(new Compartment())
  const latest = useRef({
    value,
    onChange,
    onLint,
    readOnly,
    ariaLabel,
    autoFocus,
  })
  useEffect(() => {
    latest.current = {
      value,
      onChange,
      onLint,
      readOnly,
      ariaLabel,
      autoFocus,
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
    if (current.autoFocus) view.focus()
    return () => {
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
  }))

  return (
    <div
      ref={host}
      className={cn(
        "overflow-hidden rounded-lg border border-input bg-transparent transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring dark:bg-input/30 [&_.cm-editor]:max-h-[50vh] [&_.cm-editor]:min-h-40",
        readOnly && "border-transparent bg-muted/50 dark:bg-muted/30"
      )}
    />
  )
}
