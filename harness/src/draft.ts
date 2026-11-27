import { userMessage } from "./brief/render"
import { log } from "./log"
import type { DraftApi } from "./mcp"
import type { DraftFailure, DraftJob, FinishDraftInput } from "./protocol"
import {
  CallError,
  NoFixtureError,
  type Session,
  type SessionFactory,
  type ToolResult,
  type ToolSpec,
  type Turn,
} from "./session/types"

export const draftWaitSeconds = 30
const initialBackoffMs = 1_000
const maxBackoffMs = 30_000

export type DraftOptions = {
  waitSeconds?: number
  initialBackoffMs?: number
  sleep?: (ms: number, signal: AbortSignal) => Promise<void>
}

function sleepFor(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve()
      return
    }
    const onAbort = (): void => {
      clearTimeout(timer)
      resolve()
    }
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort)
      resolve()
    }, ms)
    signal.addEventListener("abort", onAbort, { once: true })
  })
}

// The server caps drafts in flight, so the loop keeps one claim_draft waiting at all times; the queue
// counts a harness as listening only while it polls.
export async function draftLoop(
  client: DraftApi,
  factory: SessionFactory,
  signal: AbortSignal,
  options: DraftOptions = {}
): Promise<void> {
  const sleep = options.sleep ?? sleepFor
  const start = options.initialBackoffMs ?? initialBackoffMs
  const inFlight = new Set<Promise<void>>()
  let backoff = start
  while (!signal.aborted) {
    let job: DraftJob | null
    try {
      job = (
        await client.claimDraft(options.waitSeconds ?? draftWaitSeconds, signal)
      ).draft
    } catch (error) {
      if (signal.aborted) {
        break
      }
      log("warn", "claim_draft failed", {
        backoff_ms: backoff,
        error: error instanceof Error ? error.name : "unknown",
      })
      await sleep(backoff, signal)
      backoff = Math.min(backoff * 2, maxBackoffMs)
      continue
    }
    backoff = start
    if (job === null) {
      continue
    }
    const draft = job
    const task: Promise<void> = runDraft(client, draft, factory, signal)
      .catch((error: unknown) => {
        log("warn", "draft failed", {
          draft_id: draft.id,
          error: error instanceof Error ? error.name : "unknown",
        })
      })
      .finally(() => inFlight.delete(task))
    inFlight.add(task)
  }
  await Promise.allSettled(inFlight)
}

export function draftFailure(error: unknown): DraftFailure {
  if (error instanceof NoFixtureError) {
    return "no_fixture"
  }
  if (error instanceof CallError) {
    return error.failure
  }
  return "error"
}

function toolsOf(job: DraftJob): ToolSpec[] {
  return job.tools.map((t) => ({
    name: t.name,
    description: t.description,
    inputSchema: t.input_schema,
  }))
}

function parsedInput(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function turnReport(
  id: string,
  turn: Turn,
  durationMs: number
): FinishDraftInput {
  return {
    id,
    turn: {
      stop: turn.stop,
      tool_calls: turn.toolCalls.map((c) => ({
        id: c.id,
        name: c.name,
        input: parsedInput(c.input),
      })),
    },
    usage: {
      input_tokens: turn.usage.inputTokens,
      output_tokens: turn.usage.outputTokens,
      cache_read_tokens: turn.usage.cacheReadTokens,
      cache_write_tokens: turn.usage.cacheWriteTokens,
    },
    duration_ms: durationMs,
  }
}

// runDraft answers one draft turn by turn: the server checks each answer and either ends the draft or
// sends the tool results for the next turn.
export async function runDraft(
  client: DraftApi,
  job: DraftJob,
  factory: SessionFactory,
  signal: AbortSignal
): Promise<void> {
  const controller = new AbortController()
  const stop = (): void => controller.abort()
  signal.addEventListener("abort", stop, { once: true })
  const deadline = setTimeout(stop, job.timeout_seconds * 1000)
  try {
    const tools = toolsOf(job)
    let session: Session
    const started = Date.now()
    try {
      session = await factory(
        job.model,
        {
          system: job.system,
          prompt: job.prompt,
          userMessage: userMessage(job.context, job.prompt),
          buildSaves: 0,
          pruning: { keepTurns: 0, stepBytes: 0 },
        },
        controller.signal
      )
    } catch (error) {
      await client.finishDraft(
        {
          id: job.id,
          failure: draftFailure(error),
          duration_ms: Date.now() - started,
        },
        signal
      )
      return
    }
    let results: ToolResult[] | undefined
    for (;;) {
      const turnStarted = Date.now()
      let report: FinishDraftInput
      try {
        const { turn } = await session.next(results, {
          signal: controller.signal,
          tools,
          onText: () => {},
        })
        report = turnReport(job.id, turn, Date.now() - turnStarted)
      } catch (error) {
        report = {
          id: job.id,
          failure: draftFailure(error),
          duration_ms: Date.now() - turnStarted,
        }
      }
      const answer = await client.finishDraft(report, signal)
      if (answer.done || report.failure !== undefined) {
        return
      }
      results = answer.results.map((r) => ({
        callId: r.call_id,
        content: r.content,
        isError: r.is_error,
        prunable: false,
      }))
    }
  } finally {
    clearTimeout(deadline)
    signal.removeEventListener("abort", stop)
  }
}
