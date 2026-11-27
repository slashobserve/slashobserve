import { useEffect, useRef, useState, type KeyboardEvent } from "react"

import { AskAnswer, AskedFrom, AskProblem } from "@/ask/ask-result"
import { AskField } from "@/ask/ask-field"
import { askModeLabel, nextMode, type InputMode } from "@/ask/input-mode"
import { InputModeToggle } from "@/ask/input-mode-toggle"
import { useAsk } from "@/ask/use-ask"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { Button } from "@/components/ui/button"
import { languageReference } from "@/datasources/sources"
import { isModEnter, modChordLabel } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { MetricsBrowser } from "@/panel-editor/metrics-browser"
import { useAgentInfo } from "@/observe/queries"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"
import type { LintResult } from "@/query-editor/server-lint"

import { syntaxProblem } from "./problems"
import { SyntaxProblemView } from "./syntax-problem"
import { useMetricsWorkspace } from "./workspace"

const helpText = "Ctrl Space completes metric names, labels and functions."
const askPlaceholder =
  "Ask in plain language, like: requests per second by service over the last hour"
const modeLabels: Record<InputMode, string> = {
  ask: askModeLabel,
  builder: "Builder",
  code: "PromQL",
}
const modes: readonly InputMode[] = ["code", "ask"]

export function QueryBar() {
  const {
    datasource,
    search,
    draft,
    setDraft,
    run,
    range,
    inputMode,
    setInputMode,
    focusQuery,
    registerQueryFocus,
  } = useMetricsWorkspace()
  const agentAvailable = useAgentInfo().data?.available === true
  const editorRef = useRef<CodeEditorHandle>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const answeredQuery = useRef<string | null>(null)
  const [lint, setLint] = useState<LintResult | null>(null)
  const [blocked, setBlocked] = useState<string | null>(null)
  const [browsing, setBrowsing] = useState(false)
  const empty = draft.trim() === ""
  const reference = languageReference("metrics", datasource.type)
  const problem = blocked === draft ? syntaxProblem(lint, draft) : null

  const ask = useAsk({
    uid: datasource.uid,
    signal: "metrics",
    range,
    initialQuestion: search.ask,
    initialFromQuestion:
      search.q !== undefined && search.ask !== undefined ? search.ask : null,
    onAnswer: (drafted, question) => {
      answeredQuery.current = drafted.query
      setDraft(drafted.query)
      run(drafted.query, question)
    },
    onCannotTranslate: () => switchMode("code", false),
  })
  const asking = inputMode === "ask"
  const answered = ask.answer !== null || ask.fromQuestion !== null
  const editorShown = asking ? answered : true

  function switchMode(next: InputMode, remember = true) {
    if (next === inputMode) return
    setInputMode(next, remember)
    focusQuery()
  }

  useInputModeShortcut(() => switchMode(nextMode(inputMode, agentAvailable)))

  useEffect(() => {
    registerQueryFocus(() => {
      if (asking) askFieldRef.current?.focus()
      else editorRef.current?.focusAt(Number.MAX_SAFE_INTEGER)
    })
  }, [asking, registerQueryFocus])

  function onDraftChange(next: string) {
    setDraft(next)
    if (asking && ask.answer !== null && next !== answeredQuery.current) {
      setInputMode("code", false)
    }
  }

  function submit() {
    if (empty) return
    ask.clearProblem()
    if (syntaxProblem(lint, draft) !== null) {
      setBlocked(draft)
      return
    }
    setBlocked(null)
    editorRef.current?.closeCompletions()
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
      <div className="px-4 pt-3">
        <InputModeToggle
          mode={inputMode}
          modes={modes}
          labels={modeLabels}
          agentAvailable={agentAvailable}
          onChange={(next) => switchMode(next)}
        />
      </div>
      <div className="flex flex-col gap-2.5 px-4 pt-3 pb-4">
        {asking ? (
          <AskField
            ref={askFieldRef}
            signal="metrics"
            value={ask.question}
            onChange={ask.setQuestion}
            onAsk={ask.ask}
            pending={ask.asking}
            onCancel={ask.cancel}
            placeholder={askPlaceholder}
          />
        ) : null}
        {asking && ask.answer !== null ? (
          <AskAnswer answer={ask.answer} />
        ) : null}
        {ask.problem !== null ? <AskProblem problem={ask.problem} /> : null}
        {browsing && !asking ? (
          <MetricsBrowser
            datasourceUid={datasource.uid}
            datasourceType={datasource.type}
            onInsert={(text) => editorRef.current?.insertAtCursor(text)}
          />
        ) : null}
        {editorShown ? (
          <div
            onKeyDownCapture={onKeyDown}
            className={cn(
              "[&>div]:bg-background",
              asking && "[&_.cm-editor]:max-h-44!"
            )}
          >
            <CodeEditor
              value={draft}
              onChange={onDraftChange}
              datasource={{ type: datasource.type, uid: datasource.uid }}
              readOnly={false}
              ariaLabel="PromQL query"
              onLint={setLint}
              editorRef={editorRef}
            />
          </div>
        ) : null}
        {problem === null || !editorShown ? null : (
          <SyntaxProblemView problem={problem} />
        )}
        {asking ? null : (
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
        )}
      </div>
      {editorShown ? (
        <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-2 border-t px-4 py-2.5">
          {asking ? null : (
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
          )}
          <span className="text-xs text-muted-foreground">
            or press {modChordLabel(navigator.userAgent, "")}Enter
          </span>
          {!asking && ask.fromQuestion !== null ? (
            <AskedFrom
              question={ask.fromQuestion}
              onWriteAgain={() => {
                ask.askAgain()
                switchMode("ask")
              }}
            />
          ) : null}
          {asking ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="narrow:h-11"
              onClick={() => switchMode("code")}
            >
              Edit as PromQL
            </Button>
          ) : null}
          <Button
            type="button"
            variant={asking ? "outline" : "default"}
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
      ) : null}
    </>
  )
}
