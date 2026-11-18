import { useEffect, useRef, useState, type KeyboardEvent } from "react"

import { Button } from "@/components/ui/button"
import { languageReference } from "@/datasources/sources"
import { isModEnter, modChordLabel } from "@/lib/shortcuts"
import { MetricsBrowser } from "@/panel-editor/metrics-browser"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"
import type { LintResult } from "@/query-editor/server-lint"

import { syntaxProblem } from "./problems"
import { useMetricsWorkspace } from "./workspace"

const helpText = "Ctrl Space completes metric names, labels and functions."

export function QueryBar() {
  const { datasource, draft, setDraft, run, registerQueryFocus } =
    useMetricsWorkspace()
  const editorRef = useRef<CodeEditorHandle>(null)
  const [lint, setLint] = useState<LintResult | null>(null)
  const [blocked, setBlocked] = useState<string | null>(null)
  const [browsing, setBrowsing] = useState(false)
  const empty = draft.trim() === ""
  const reference = languageReference("metrics", datasource.type)
  const problem = blocked === draft ? syntaxProblem(lint, draft) : null

  useEffect(() => {
    registerQueryFocus(() =>
      editorRef.current?.focusAt(Number.MAX_SAFE_INTEGER)
    )
  }, [registerQueryFocus])

  function submit() {
    if (empty) return
    if (syntaxProblem(lint, draft) !== null) {
      setBlocked(draft)
      return
    }
    setBlocked(null)
    run()
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.stopPropagation()
    submit()
  }

  return (
    <>
      <div className="flex flex-col gap-2.5 px-4 pt-3 pb-4">
        {browsing ? (
          <MetricsBrowser
            datasourceUid={datasource.uid}
            datasourceType={datasource.type}
            onInsert={(text) => editorRef.current?.insertAtCursor(text)}
          />
        ) : null}
        <div onKeyDownCapture={onKeyDown} className="[&>div]:bg-background">
          <CodeEditor
            value={draft}
            onChange={setDraft}
            datasource={{ type: datasource.type, uid: datasource.uid }}
            readOnly={false}
            ariaLabel="PromQL query"
            onLint={setLint}
            editorRef={editorRef}
          />
        </div>
        {problem === null ? null : (
          <div role="alert" className="flex flex-col gap-0.5">
            <p className="text-xs text-destructive">
              Line {problem.line}, column {problem.column}: {problem.message}
            </p>
            <pre className="m-0 overflow-x-auto font-mono text-xs">
              {problem.source}
              {"\n"}
              <span className="text-destructive">{problem.caret}</span>
            </pre>
            <p className="text-xs text-muted-foreground">
              Nothing ran. Fix the query and run it again.
            </p>
          </div>
        )}
        <p className="text-xs text-muted-foreground">
          {helpText}{" "}
          {reference === null ? null : (
            <a
              href={reference.url}
              target="_blank"
              rel="noreferrer"
              className="underline underline-offset-3"
            >
              {reference.label}
            </a>
          )}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-2 border-t px-4 py-2.5">
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-expanded={browsing}
          className="mr-auto narrow:h-11"
          onClick={() => setBrowsing((open) => !open)}
        >
          {browsing ? "Hide metrics" : "Browse metrics"}
        </Button>
        <span className="text-xs text-muted-foreground">
          or press {modChordLabel(navigator.userAgent, "")}Enter
        </span>
        <Button
          type="button"
          size="sm"
          disabled={empty}
          title={empty ? "Write a query first." : "Run query"}
          aria-keyshortcuts="Control+Enter Meta+Enter"
          className="narrow:h-11"
          onClick={submit}
        >
          Run query
        </Button>
      </div>
    </>
  )
}
