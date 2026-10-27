import { useId, useRef, useState, type Dispatch, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAgentInfo } from "@/observe/queries"

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

type Started = { suggestionId: string; panel: EditablePanel; prompt: string }

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
  const [started, setStarted] = useState<Started | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [applied, setApplied] = useState("")
  const tracked = useQuery({
    ...suggestionQuery(dashboardId, started?.suggestionId ?? ""),
    enabled: started !== null,
  })

  if (agent.data?.available === false) return null

  const status = tracked.data?.status
  const busy =
    create.isPending ||
    (started !== null &&
      status !== undefined &&
      status !== "succeeded" &&
      status !== "failed")

  async function send(text: string) {
    const trimmed = text.trim()
    const problem = promptProblem(trimmed)
    setApplied("")
    if (problem !== null) {
      setError(problem)
      return
    }
    setError(null)
    const panel = structuredClone(draft)
    try {
      const suggestion = await create.mutateAsync({
        version,
        panel_id: draft.id,
        prompt: trimmed,
        panel: draft,
      })
      setStarted({ suggestionId: suggestion.id, panel, prompt: trimmed })
    } catch (failure) {
      setError(describeSuggestionError(failure))
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
    setStarted(null)
    setApplied(appliedMessage)
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
          aria-invalid={error !== null && started === null}
          className="h-7 border-0 bg-transparent px-0 focus-visible:ring-0"
          onChange={(event) => {
            setPrompt(event.target.value)
            setError(null)
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
          onDiscard={() => setStarted(null)}
          onRetry={() => void send(started.prompt)}
        />
      )}
      <p role="status" className="sr-only">
        {applied}
      </p>
    </div>
  )
}
