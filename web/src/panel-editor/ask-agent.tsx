import {
  useId,
  useReducer,
  useRef,
  useState,
  type Dispatch,
  type FormEvent,
} from "react"
import { useQuery } from "@tanstack/react-query"

import { isRetryable } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAgentInfo } from "@/observe/queries"

import { askIdle, reduceAsk, suggestBusy } from "./ask-agent-state"
import { rebasePanel } from "./changes"
import type { DraftAction, EditablePanel } from "./draft"
import {
  describeSuggestionError,
  suggestionQuery,
  useCreateSuggestion,
} from "./queries"
import { SuggestionCard } from "./suggestion-card"

export type AskAgentProps = {
  dashboardId: string
  version: number
  draft: EditablePanel
  dispatch: Dispatch<DraftAction>
  blockedReason: string | null
}

const maxPromptCodePoints = 4000
const placeholder =
  "Ask the agent to change this query, like: only 5xx, by route"
const appliedMessage = "Applied the agent's change. Save to keep it."

function promptProblem(prompt: string): string | null {
  if (prompt === "") return "Describe the change you want first."
  if (Array.from(prompt).length > maxPromptCodePoints) {
    return `Use at most ${maxPromptCodePoints} characters.`
  }
  return null
}

export function AskAgent({
  dashboardId,
  version,
  draft,
  dispatch,
  blockedReason,
}: AskAgentProps) {
  const inputId = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const agent = useAgentInfo()
  const create = useCreateSuggestion(dashboardId)
  const [prompt, setPrompt] = useState("")
  const [phase, act] = useReducer(reduceAsk, askIdle)
  const creating = useRef(false)
  const started = phase.kind === "tracking" ? phase.started : null
  const tracked = useQuery({
    ...suggestionQuery(dashboardId, started?.suggestionId ?? ""),
    enabled: started !== null,
  })
  const busy = suggestBusy(phase, tracked.data?.status)
  if (
    started !== null &&
    busy &&
    tracked.isError &&
    !isRetryable(tracked.error)
  ) {
    act({
      type: "poll_failed",
      suggestionId: started.suggestionId,
      error: describeSuggestionError(tracked.error),
    })
  }

  if (agent.data?.available === false) return null

  const error = phase.kind === "failed" ? phase.error : null

  async function send(text: string) {
    if (creating.current) return
    const trimmed = text.trim()
    const problem = promptProblem(trimmed)
    if (problem !== null) {
      act({ type: "refused", error: problem })
      return
    }
    creating.current = true
    const panel = structuredClone(draft)
    act({ type: "create_started", panel, prompt: trimmed })
    try {
      const suggestion = await create.mutateAsync({
        version,
        panel_id: draft.id,
        prompt: trimmed,
        panel: draft,
      })
      act({ type: "created", suggestionId: suggestion.id })
    } catch (failure) {
      act({ type: "create_failed", error: describeSuggestionError(failure) })
    } finally {
      creating.current = false
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!busy) void send(prompt)
  }

  function accept(proposed: EditablePanel) {
    if (started === null) return
    dispatch({
      type: "replace",
      panel: rebasePanel(started.panel, proposed, draft),
    })
    act({ type: "accepted" })
    inputRef.current?.focus()
  }

  return (
    <div className="flex flex-col gap-2">
      <form
        onSubmit={submit}
        className="flex items-center gap-2 rounded-lg border py-1.5 pr-1.5 pl-3 focus-within:border-ring focus-within:ring-2 focus-within:ring-ring"
      >
        <Label htmlFor={inputId} className="sr-only">
          Ask the agent to change this query
        </Label>
        <Input
          id={inputId}
          ref={inputRef}
          value={prompt}
          placeholder={placeholder}
          disabled={blockedReason !== null}
          aria-invalid={phase.kind === "failed" && phase.started === null}
          className="h-7 border-0 bg-transparent px-0 focus-visible:ring-0"
          onChange={(event) => {
            setPrompt(event.target.value)
            act({ type: "typed" })
          }}
        />
        <Button
          type="submit"
          variant="outline"
          size="sm"
          className="text-xs font-medium"
          disabled={blockedReason !== null || busy}
          aria-busy={busy}
        >
          Suggest
        </Button>
      </form>
      {error === null ? null : (
        <p role="alert" className="text-xs text-status-critical">
          {error}
        </p>
      )}
      {blockedReason === null ? null : (
        <p className="text-xs text-muted-foreground">{blockedReason}</p>
      )}
      {started === null || blockedReason !== null ? null : (
        <SuggestionCard
          key={started.suggestionId}
          dashboardId={dashboardId}
          suggestionId={started.suggestionId}
          started={started.panel}
          onAccept={accept}
          onDiscard={() => act({ type: "discarded" })}
          onRetry={() => void send(started.prompt)}
        />
      )}
      <p role="status" className="sr-only">
        {phase.kind === "applied" ? appliedMessage : ""}
      </p>
    </div>
  )
}
