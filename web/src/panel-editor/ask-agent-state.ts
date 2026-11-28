import type { EditablePanel } from "./draft"

type Started = {
  suggestionId: string
  panel: EditablePanel
  prompt: string
}

export type AskPhase =
  | { kind: "idle" }
  | { kind: "creating"; panel: EditablePanel; prompt: string }
  | { kind: "tracking"; started: Started }
  | { kind: "applied" }
  | { kind: "failed"; error: string; started: Started | null }

export type AskEvent =
  | { type: "refused"; error: string }
  | { type: "create_started"; panel: EditablePanel; prompt: string }
  | { type: "created"; suggestionId: string }
  | { type: "create_failed"; error: string }
  | { type: "poll_failed"; suggestionId: string; error: string }
  | { type: "accepted" }
  | { type: "discarded" }
  | { type: "typed" }

export const askIdle: AskPhase = { kind: "idle" }

type SuggestionStatus = "queued" | "running" | "succeeded" | "failed"

export function suggestBusy(
  phase: AskPhase,
  status: SuggestionStatus | undefined
): boolean {
  if (phase.kind === "creating") return true
  return (
    phase.kind === "tracking" && status !== "succeeded" && status !== "failed"
  )
}

export function reduceAsk(phase: AskPhase, event: AskEvent): AskPhase {
  switch (event.type) {
    case "refused":
      return phase.kind === "creating"
        ? phase
        : { kind: "failed", error: event.error, started: null }
    case "create_started":
      return phase.kind === "creating"
        ? phase
        : { kind: "creating", panel: event.panel, prompt: event.prompt }
    case "created":
      return phase.kind === "creating"
        ? {
            kind: "tracking",
            started: {
              suggestionId: event.suggestionId,
              panel: phase.panel,
              prompt: phase.prompt,
            },
          }
        : phase
    case "create_failed":
      return phase.kind === "creating"
        ? { kind: "failed", error: event.error, started: null }
        : phase
    case "poll_failed":
      return phase.kind === "tracking" &&
        phase.started.suggestionId === event.suggestionId
        ? { kind: "failed", error: event.error, started: phase.started }
        : phase
    case "accepted":
      return phase.kind === "tracking" ? { kind: "applied" } : phase
    case "discarded":
      return phase.kind === "tracking" ? askIdle : phase
    case "typed":
      return phase.kind === "failed" ? askIdle : phase
  }
}
