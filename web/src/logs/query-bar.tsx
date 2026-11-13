import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { AskField } from "@/ask/ask-field"
import { nextMode, type InputMode } from "@/ask/input-mode"
import { useAsk } from "@/ask/use-ask"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { PanelDataResponse } from "@/dashboards/queries"
import { formatDateTime } from "@/lib/time"
import { isModEnter, modChordLabel } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"
import type { LintResult } from "@/query-editor/server-lint"

import { BuilderBar } from "./builder-bar"
import { canBuild, withoutField } from "./builder"
import { typeName } from "./datasource-picker"
import {
  DiscoverUrlError,
  importDiscoverUrl,
  importSummary,
  isDiscoverUrl,
} from "./discover-url"
import { parseKql } from "./kql"
import { LogLabelBrowser } from "./label-browser"
import { addPill, decodePills, encodePills, unpinnedCleared } from "./pills"
import { formLabels, formOf, splitForm, withForm } from "./query-form"
import { problemOf, type QueryProblem } from "./query-problem"
import { kqlReferenceUrl, QueryProblemCard } from "./query-problem.tsx"
import { isSearchFamily, queryModeLabel, type LogsSearch } from "./search"
import { useLogsWorkspace } from "./workspace"

const askPlaceholder =
  "Ask in plain language, like: errors from checkout in the last hour with a trace id"
const noModelText = "Add a model key to ask in plain language"
const builderBlockedText = "The builder cannot show this query. Edit it as KQL."
const unknownFieldMessage = /^unknown field "(.*)"$/
const segmentClass =
  "inline-flex h-full items-center rounded-control border px-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
const referenceUrls: Record<string, string> = {
  KQL: kqlReferenceUrl,
  LogQL: "https://grafana.com/docs/loki/latest/query/",
  SQL: "https://clickhouse.com/docs/sql-reference",
}
const formItems = (["kql", "lucene", "dsl"] as const).map((value) => ({
  value,
  label: formLabels[value],
}))

function useBarMode() {
  const { datasource, draft, mode, setMode, labels } = useLogsWorkspace()
  const agentAvailable = useAgentInfo().data?.available === true
  const type = datasource.type
  const search = isSearchFamily(type)
  const builderOffered = type !== "clickhouse"
  const builderAvailable = search ? canBuild(draft) : type === "loki"
  const modes: InputMode[] = builderOffered
    ? ["builder", "code", "ask"]
    : ["code", "ask"]
  let shown: InputMode = mode
  if (type === "loki") {
    shown = mode === "ask" ? "ask" : labels.open ? "builder" : "code"
  } else if (mode === "builder" && !builderAvailable) {
    shown = "code"
  }

  function available(candidate: InputMode): boolean {
    if (candidate === "ask") return agentAvailable
    if (candidate === "builder") return builderAvailable
    return true
  }

  function select(next: InputMode, remember = true) {
    if (!available(next)) return
    if (type === "loki") {
      if (next === "builder") {
        labels.setOpen(true)
        if (mode === "ask") setMode("code", remember)
        return
      }
      if (next === "code") labels.setOpen(false)
    }
    if (next !== mode) setMode(next, remember)
  }

  return {
    type,
    search,
    shown,
    modes,
    agentAvailable,
    builderAvailable,
    available,
    select,
  }
}

function ModeSwitch() {
  const { focusQuery } = useLogsWorkspace()
  const { type, search, shown, modes, available, select } = useBarMode()
  const blockedId = useId()
  const buttons = useRef<Partial<Record<InputMode, HTMLButtonElement | null>>>(
    {}
  )
  const words: Record<InputMode, string> = {
    builder: "Builder",
    code: queryModeLabel(type),
    ask: "Ask",
  }

  function choose(next: InputMode, from: "click" | "key") {
    if (!available(next)) return
    select(next)
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
    const reachable = modes.filter(available)
    const index = reachable.indexOf(shown)
    choose(
      reachable[(index + step + reachable.length) % reachable.length],
      "key"
    )
  }

  function segment(value: InputMode) {
    const selected = shown === value
    const unavailable = !available(value)
    const blocked =
      value === "builder" && search ? builderBlockedText : undefined
    const button = (
      <button
        ref={(element) => {
          buttons.current[value] = element
        }}
        type="button"
        role="radio"
        aria-checked={selected}
        aria-disabled={unavailable || undefined}
        aria-describedby={
          blocked === undefined || !unavailable ? undefined : blockedId
        }
        title={value === "ask" && unavailable ? noModelText : undefined}
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
        {words[value]}
      </button>
    )
    if (!unavailable || blocked === undefined) return button
    return (
      <Tooltip>
        <TooltipTrigger render={button} />
        <TooltipContent>{blocked}</TooltipContent>
      </Tooltip>
    )
  }

  return (
    <div
      role="radiogroup"
      aria-label="Query mode"
      aria-keyshortcuts="Control+I Meta+I"
      className="inline-flex h-8 items-center gap-0.5 rounded-control bg-muted p-0.5"
    >
      {modes.map((value) => (
        <span key={value} className="contents">
          {segment(value)}
        </span>
      ))}
      {search && !available("builder") ? (
        <span id={blockedId} className="sr-only">
          {builderBlockedText}
        </span>
      ) : null}
    </div>
  )
}

function lastGoodRun(
  queryClient: ReturnType<typeof useQueryClient>,
  uid: string
): { at: number; rows: number } | null {
  let best: { at: number; rows: number } | null = null
  for (const query of queryClient
    .getQueryCache()
    .findAll({ queryKey: ["explore", uid, "query"] })) {
    const data = query.state.data as PanelDataResponse | undefined
    if (query.state.status !== "success" || data === undefined) continue
    if (best === null || query.state.dataUpdatedAt > best.at) {
      best = { at: query.state.dataUpdatedAt, rows: data.meta.rows }
    }
  }
  return best
}

function RunFailure({ uid, search }: { uid: string; search: boolean }) {
  const queryClient = useQueryClient()
  const good = lastGoodRun(queryClient, uid)
  return (
    <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
      {good === null
        ? "No results."
        : `No results. The last good run was ${formatDateTime(good.at)}, ${good.rows.toLocaleString("en")} ${search ? "hits" : "lines"}.`}
      {search ? (
        <Link to="/datasources" className="underline underline-offset-3">
          Check the datasource
        </Link>
      ) : null}
    </p>
  )
}

export function QueryBar() {
  const {
    datasource,
    search: urlSearch,
    update,
    range,
    draft,
    setDraft,
    run,
    mode,
    setMode,
    focusQuery,
    registerQueryFocus,
    results,
  } = useLogsWorkspace()
  const uid = datasource.uid
  const type = datasource.type
  const modeLabel = queryModeLabel(type)
  const bar = useBarMode()
  const { shown, search, builderAvailable, agentAvailable } = bar
  const editorRef = useRef<CodeEditorHandle>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const labelsRef = useRef<HTMLDivElement>(null)
  const answeredQuery = useRef<string | null>(null)
  const labelsWereShown = useRef(false)
  const [blocked, setBlocked] = useState<string | null>(null)
  const [lint, setLint] = useState<LintResult | null>(null)
  const [discoverNote, setDiscoverNote] = useState<string | null>(null)
  const emptyDraft = draft.trim() === ""
  const form = formOf(draft)

  function switchMode(next: InputMode, remember = true) {
    if (next === shown) return
    bar.select(next, remember)
    focusQuery()
  }

  const ask = useAsk({
    uid,
    signal: "logs",
    range,
    initialQuestion: urlSearch.ask,
    initialFromQuestion:
      urlSearch.q !== undefined && urlSearch.ask !== undefined
        ? urlSearch.ask
        : null,
    onAnswer: (drafted, question) => {
      answeredQuery.current = drafted.query
      setDraft(drafted.query)
      run(drafted.query, question)
    },
    onCannotTranslate: () => switchMode("code", false),
  })
  const answered = ask.answer !== null || ask.fromQuestion !== null
  const builderInBar = shown === "builder" && search
  const labelsShown = shown === "builder" && type === "loki"
  const editorShown = shown === "ask" ? answered : !builderInBar
  const toolbarShown = shown === "ask" ? answered : true

  const problem = useMemo<QueryProblem | null>(() => {
    if (!search || blocked !== draft) return null
    const { form: current, body, offset } = splitForm(draft)
    if (current !== "kql") return null
    return problemOf(draft, parseKql(body), offset)
  }, [search, blocked, draft])

  useInputModeShortcut(() =>
    switchMode(nextMode(shown, agentAvailable, builderAvailable))
  )

  useEffect(() => {
    registerQueryFocus(() => {
      if (shown === "ask") askFieldRef.current?.focus()
      else editorRef.current?.focusAt(Number.MAX_SAFE_INTEGER)
    })
  }, [shown, registerQueryFocus])

  useEffect(() => {
    if (labelsShown && !labelsWereShown.current) labelsRef.current?.focus()
    labelsWereShown.current = labelsShown
  }, [labelsShown])

  function onDraftChange(next: string) {
    setDraft(next)
    if (
      mode === "ask" &&
      ask.answer !== null &&
      next !== answeredQuery.current
    ) {
      setMode("code", false)
    }
  }

  function submit() {
    if (emptyDraft) return
    ask.clearProblem()
    if (search && formOf(draft) === "kql") {
      const { body, offset } = splitForm(draft)
      if (problemOf(draft, parseKql(body), offset) !== null) {
        setBlocked(draft)
        return
      }
    }
    setBlocked(null)
    run()
  }

  function onEditorKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.stopPropagation()
    submit()
  }

  function onEditorPaste(event: ClipboardEvent<HTMLDivElement>) {
    if (!search) return
    const text = event.clipboardData.getData("text/plain")
    if (!isDiscoverUrl(text)) return
    event.preventDefault()
    event.stopPropagation()
    let imported: ReturnType<typeof importDiscoverUrl>
    try {
      imported = importDiscoverUrl(text)
    } catch (error) {
      if (!(error instanceof DiscoverUrlError)) throw error
      setDiscoverNote("That Discover link could not be read.")
      return
    }
    const kept = decodePills(urlSearch.f)
    const pills = imported.pills.reduce(
      (current, pill) => addPill(current, pill),
      imported.q === undefined ? kept : unpinnedCleared(kept)
    )
    const next: Partial<LogsSearch> = {}
    if (imported.q !== undefined) next.q = imported.q
    if (imported.from !== undefined) next.from = imported.from
    if (imported.to !== undefined) next.to = imported.to
    if (imported.cols !== undefined) next.cols = imported.cols.join(",")
    if (imported.pills.length > 0) next.f = encodePills(pills)
    if (imported.q !== undefined) setDraft(imported.q)
    update(next)
    if (imported.q !== undefined) run(imported.q)
    setDiscoverNote(importSummary(imported))
  }

  function insert(text: string) {
    if (mode === "ask") setMode("code", false)
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

  function fixWithAsk(question: string) {
    ask.setQuestion(question)
    switchMode("ask")
  }

  const unknownFields =
    lint?.status === "ok" && lint.query === draft
      ? lint.items.flatMap((item) => {
          const match = unknownFieldMessage.exec(item.message)
          return match === null ? [] : [match[1]]
        })
      : []

  return (
    <div className="flex flex-col gap-3">
      <div data-slot="builder" className="empty:hidden">
        {builderInBar ? <BuilderBar /> : null}
      </div>
      {shown === "ask" ? (
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
      {shown === "ask" && ask.answer !== null ? (
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
      {labelsShown && editorShown ? (
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
        <div className="flex flex-col gap-1">
          <div
            onKeyDownCapture={onEditorKeyDown}
            onPasteCapture={onEditorPaste}
            className={cn(shown === "ask" && "[&_.cm-editor]:max-h-44!")}
          >
            <CodeEditor
              value={draft}
              onChange={onDraftChange}
              datasource={{ type, uid }}
              readOnly={false}
              ariaLabel={`${modeLabel} query`}
              onLint={setLint}
              editorRef={editorRef}
            />
          </div>
          {problem !== null ? (
            <div role="status" className="flex flex-col gap-0.5">
              <pre className="m-0 font-mono text-xs text-destructive">
                {problem.caret}
              </pre>
              <p className="text-xs text-muted-foreground">
                Nothing ran. Checked by the KQL parser before {typeName(type)}{" "}
                is asked
              </p>
            </div>
          ) : null}
          {discoverNote === null ? null : (
            <p
              role="status"
              className="flex items-center gap-2 text-xs text-muted-foreground"
            >
              {discoverNote}
              <Button
                type="button"
                variant="link"
                size="xs"
                onClick={() => setDiscoverNote(null)}
              >
                Dismiss
              </Button>
            </p>
          )}
          {unknownFields.map((field) => (
            <UnknownField
              key={field}
              field={field}
              index={datasource.index}
              onRemove={() => {
                const next = withoutField(draft, field)
                if (next !== null) setDraft(next)
              }}
              removable={withoutField(draft, field) !== null}
            />
          ))}
          {shown === "ask" ? null : (
            <p className="text-xs text-muted-foreground">
              {helpText(type, datasource.index)}{" "}
              <a
                href={referenceUrls[modeLabel]}
                target="_blank"
                rel="noreferrer"
                className="underline underline-offset-3"
              >
                {modeLabel} reference
              </a>
            </p>
          )}
        </div>
      ) : null}
      <div data-slot="query-problem" className="empty:hidden">
        {problem !== null ? (
          <QueryProblemCard
            problem={problem}
            text={draft}
            agentAvailable={agentAvailable}
            onFixWithAsk={fixWithAsk}
          />
        ) : null}
      </div>
      {toolbarShown ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant={shown === "ask" ? "outline" : "default"}
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
          <span
            data-slot="language"
            className="ml-auto flex items-center gap-2 empty:hidden"
          >
            {search && !builderInBar ? (
              <>
                {problem?.luceneSwitch === true ? (
                  <Button
                    type="button"
                    variant="link"
                    size="xs"
                    onClick={() => {
                      const next = withForm(draft, "lucene")
                      setDraft(next)
                      run(next)
                    }}
                  >
                    Switch to Lucene
                  </Button>
                ) : null}
                <Select
                  items={formItems}
                  value={form}
                  onValueChange={(next) => {
                    if (next !== null) setDraft(withForm(draft, next))
                  }}
                >
                  <SelectTrigger size="sm" aria-label="Language">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false} align="end">
                    {formItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </>
            ) : null}
          </span>
          {shown === "ask" ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => switchMode("code")}
            >
              Edit as {modeLabel}
            </Button>
          ) : null}
          {shown !== "ask" && ask.fromQuestion !== null ? (
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
      {results.isError && !problem ? (
        <RunFailure uid={uid} search={search} />
      ) : null}
    </div>
  )
}

function helpText(type: string, index: string | undefined): string {
  if (isSearchFamily(type)) {
    return `Ctrl Space completes field names from the ${index === undefined ? "" : index + " "}mapping.`
  }
  if (type === "loki") {
    return "Ctrl Space completes stream labels and, after a parser stage, parsed fields."
  }
  return "Ctrl Space completes keywords and columns."
}

function UnknownField({
  field,
  index,
  removable,
  onRemove,
}: {
  field: string
  index: string | undefined
  removable: boolean
  onRemove: () => void
}) {
  return (
    <p className="flex flex-wrap items-center gap-2 text-xs">
      <StatusLabel tone="warning">
        {field} is not in the {index ?? "data view"} mapping
      </StatusLabel>
      {removable ? (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="link"
                size="xs"
                onClick={onRemove}
              />
            }
          >
            Remove this clause
          </TooltipTrigger>
          <TooltipContent>Removes every clause on {field}</TooltipContent>
        </Tooltip>
      ) : null}
    </p>
  )
}

QueryBar.ModeSwitch = ModeSwitch
