import { useEffect, useRef, type KeyboardEvent } from "react"

import { AskField } from "@/ask/ask-field"
import { nextMode, type InputMode } from "@/ask/input-mode"
import { useAsk } from "@/ask/use-ask"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { isModEnter, modChordLabel } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"

import { LogLabelBrowser } from "./label-browser"
import { queryModeLabel } from "./search"
import { useLogsWorkspace } from "./workspace"

const askPlaceholder =
  "Ask in plain language, like: errors from checkout in the last hour with a trace id"
const noModelText = "Add a model key to ask in plain language"
const builderText = "Builder arrives with the next plan"
const modeOrder: InputMode[] = ["query", "ask"]
const segmentClass =
  "inline-flex h-full items-center rounded-control border px-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"

function ignoreLint() {}

function ModeSwitch() {
  const { datasource, mode, setMode, focusQuery } = useLogsWorkspace()
  const agentAvailable = useAgentInfo().data?.available === true
  const buttons = useRef<Record<InputMode, HTMLButtonElement | null>>({
    ask: null,
    query: null,
  })

  function choose(next: InputMode, from: "click" | "key") {
    if (next === "ask" && !agentAvailable) return
    if (next !== mode) setMode(next)
    if (from === "click") focusQuery()
    else buttons.current[next]?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const step =
      event.key === "ArrowRight" || event.key === "ArrowDown"
        ? 1
        : event.key === "ArrowLeft" || event.key === "ArrowUp"
          ? -1
          : 0
    if (step === 0) return
    event.preventDefault()
    const index = modeOrder.indexOf(mode)
    choose(
      modeOrder[(index + step + modeOrder.length) % modeOrder.length],
      "key"
    )
  }

  function segment(value: InputMode, word: string) {
    const selected = mode === value
    const unavailable = value === "ask" && !agentAvailable
    return (
      <button
        ref={(element) => {
          buttons.current[value] = element
        }}
        type="button"
        role="radio"
        aria-checked={selected}
        aria-disabled={unavailable || undefined}
        title={unavailable ? noModelText : undefined}
        tabIndex={selected ? 0 : -1}
        onClick={() => choose(value, "click")}
        onKeyDown={onKeyDown}
        className={cn(
          segmentClass,
          selected
            ? "border-border bg-background text-foreground"
            : "border-transparent text-muted-foreground",
          unavailable && "cursor-not-allowed opacity-60"
        )}
      >
        {word}
      </button>
    )
  }

  return (
    <div
      role="radiogroup"
      aria-label="Query mode"
      aria-keyshortcuts="Control+I Meta+I"
      className="inline-flex h-8 items-center gap-0.5 rounded-control bg-muted p-0.5"
    >
      <button
        type="button"
        role="radio"
        aria-checked={false}
        disabled
        title={builderText}
        tabIndex={-1}
        className={cn(
          segmentClass,
          "cursor-not-allowed border-transparent text-muted-foreground opacity-60"
        )}
      >
        Builder
      </button>
      {segment("query", queryModeLabel(datasource.type))}
      {segment("ask", "Ask")}
    </div>
  )
}

export function QueryBar() {
  const {
    datasource,
    search,
    range,
    draft,
    setDraft,
    run,
    mode,
    setMode,
    focusQuery,
    registerQueryFocus,
    labels,
  } = useLogsWorkspace()
  const uid = datasource.uid
  const type = datasource.type
  const modeLabel = queryModeLabel(type)
  const agentAvailable = useAgentInfo().data?.available === true
  const editorRef = useRef<CodeEditorHandle>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const labelsRef = useRef<HTMLDivElement>(null)
  const answeredQuery = useRef<string | null>(null)
  const labelsWereOpen = useRef(labels.open)
  const emptyDraft = draft.trim() === ""
  function switchMode(next: InputMode, remember = true) {
    if (next === mode) return
    setMode(next, remember)
    focusQuery()
  }

  const ask = useAsk({
    uid,
    signal: "logs",
    range,
    initialQuestion: search.ask,
    initialFromQuestion:
      search.q !== undefined && search.ask !== undefined ? search.ask : null,
    onAnswer: (drafted, question) => {
      answeredQuery.current = drafted.query
      setDraft(drafted.query)
      run(drafted.query, question)
    },
    onCannotTranslate: () => switchMode("query", false),
  })
  const answered = ask.answer !== null || ask.fromQuestion !== null
  const editorShown = mode === "query" || answered

  useInputModeShortcut(() => switchMode(nextMode(mode, agentAvailable)))

  useEffect(() => {
    registerQueryFocus(() => {
      if (mode === "ask") askFieldRef.current?.focus()
      else editorRef.current?.focusAt(Number.MAX_SAFE_INTEGER)
    })
  }, [mode, registerQueryFocus])

  useEffect(() => {
    if (labels.open && !labelsWereOpen.current) labelsRef.current?.focus()
    labelsWereOpen.current = labels.open
  }, [labels.open])

  function onDraftChange(next: string) {
    setDraft(next)
    if (
      mode === "ask" &&
      ask.answer !== null &&
      next !== answeredQuery.current
    ) {
      setMode("query", false)
    }
  }

  function submit() {
    if (emptyDraft) return
    ask.clearProblem()
    run()
  }

  function onEditorKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.stopPropagation()
    submit()
  }

  function insert(text: string) {
    if (mode === "ask") setMode("query", false)
    if (type === "loki" && emptyDraft) {
      setDraft(`{${text}}`)
      return
    }
    if (editorRef.current === null) setDraft(draft + text)
    else editorRef.current.insertAtCursor(text)
  }

  function askAgain() {
    ask.askAgain()
    switchMode("ask")
  }

  return (
    <div className="flex flex-col gap-3">
      <div data-slot="builder" className="empty:hidden" />
      {mode === "ask" ? (
        <div className="[&_textarea]:pr-2.5">
          <AskField
            ref={askFieldRef}
            signal="logs"
            value={ask.question}
            onChange={ask.setQuestion}
            onAsk={ask.ask}
            pending={ask.asking}
            onCancel={ask.cancel}
            placeholder={askPlaceholder}
          />
        </div>
      ) : null}
      {mode === "ask" && ask.answer !== null ? (
        <div className="flex flex-col gap-1">
          {ask.answer.explanation !== "" ? (
            <p className="text-xs text-muted-foreground">
              {ask.answer.explanation}
            </p>
          ) : null}
          {ask.answer.warnings.map((warning) => (
            <p key={warning} className="text-xs">
              <StatusLabel tone="warning">{warning}</StatusLabel>
            </p>
          ))}
        </div>
      ) : null}
      {ask.problem !== null ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-1">
            {ask.problem.kind === "cannot_translate" ? (
              <>
                <p>I could not turn that into a query.</p>
                <p className="text-muted-foreground">{ask.problem.reason}</p>
              </>
            ) : (
              <p>{ask.problem.text}</p>
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      {labels.open && type === "loki" && editorShown ? (
        <div
          ref={labelsRef}
          tabIndex={-1}
          aria-label="Label browser"
          role="region"
          className="outline-none"
        >
          <LogLabelBrowser
            datasourceUid={uid}
            datasourceType={type}
            onInsert={insert}
          />
        </div>
      ) : null}
      {editorShown ? (
        <div
          onKeyDownCapture={onEditorKeyDown}
          className={cn(mode === "ask" && "[&_.cm-editor]:max-h-44!")}
        >
          <CodeEditor
            value={draft}
            onChange={onDraftChange}
            datasource={{ type, uid }}
            readOnly={false}
            ariaLabel="Log query"
            onLint={ignoreLint}
            editorRef={editorRef}
          />
        </div>
      ) : null}
      <div data-slot="query-problem" className="empty:hidden" />
      {editorShown ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant={mode === "ask" ? "outline" : "default"}
            disabled={emptyDraft}
            title={emptyDraft ? "Write a query first." : "Run query"}
            aria-keyshortcuts="Control+Enter Meta+Enter"
            onClick={submit}
          >
            Run query
          </Button>
          <span className="text-xs text-muted-foreground">
            or press {modChordLabel(navigator.userAgent, "")}Enter
          </span>
          {type === "loki" ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-expanded={labels.open}
              onClick={() => labels.setOpen(!labels.open)}
            >
              Labels
            </Button>
          ) : null}
          <span data-slot="language" className="ml-auto empty:hidden" />
          {mode === "ask" ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => switchMode("query")}
            >
              Edit as {modeLabel}
            </Button>
          ) : null}
          {mode === "query" && ask.fromQuestion !== null ? (
            <>
              <span className="min-w-0 truncate text-xs text-muted-foreground">
                From: {ask.fromQuestion}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={askAgain}
              >
                Ask again
              </Button>
            </>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

QueryBar.ModeSwitch = ModeSwitch
