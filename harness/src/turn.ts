import type { Stop } from "./guard"
import { type RunTools, type ToolReply } from "./mcp"
import { isSave } from "./phase"
import {
  type CallHints,
  type RunMeta,
  type TurnMeta,
  type Usage,
} from "./protocol"
import type { ToolCall, ToolResult } from "./session/types"

export const maxParallelTools = 4
export const toolTimeoutMs = 300_000

export type { Stop }

export type Executed = {
  results: ToolResult[]
  metas: RunMeta[]
  stop?: Stop | undefined
}

export type Args = Record<string, unknown>

export type Admitted =
  | { refused: true; content: string }
  | { refused: false; args: Args; hints?: CallHints | undefined }

export type Settled = { content: string; stop?: Stop | undefined }

export interface Watch {
  beginTurn(saves: number): void
  admit(call: ToolCall, args: Args): Admitted
  settle(
    call: ToolCall,
    args: Args,
    admitted: Admitted,
    reply: ToolReply
  ): Settled
  failed(call: ToolCall, admitted: Admitted): void
  endTurn(): { notes: string[]; stop?: Stop | undefined }
}

export const unwatched: Watch = {
  beginTurn: () => {},
  admit: (_call, args) => ({ refused: false, args }),
  settle: (_call, _args, _admitted, reply) => ({ content: reply.content }),
  failed: () => {},
  endTurn: () => ({ notes: [] }),
}

type Dispatch = { index: number; call: ToolCall; args: Args }

function errorResult(call: ToolCall, code: string, message: string) {
  return {
    callId: call.id,
    content: JSON.stringify({ code, message }),
    isError: true,
    prunable: false,
  } satisfies ToolResult
}

function parseArguments(input: string): Args | undefined {
  try {
    const value: unknown = JSON.parse(input)
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? (value as Args)
      : undefined
  } catch {
    return undefined
  }
}

export function withNotes(content: string, notes: readonly string[]): string {
  return notes.reduce(
    (text, note) => text + "\n\n<note>" + note + "</note>",
    content
  )
}

async function inParallel<T>(
  items: T[],
  limit: number,
  work: (item: T) => Promise<void>
): Promise<void> {
  let next = 0
  const lane = async (): Promise<void> => {
    while (next < items.length) {
      const item = items[next++] as T
      await work(item)
    }
  }
  await Promise.all(
    Array.from({ length: Math.min(limit, items.length) }, () => lane())
  )
}

export async function executeTurn(
  tools: RunTools,
  calls: ToolCall[],
  turnId: string,
  usage: Usage,
  signal: AbortSignal,
  watch: Watch = unwatched
): Promise<Executed> {
  const results: Array<ToolResult | undefined> = calls.map(() => undefined)
  const metaAt: Array<RunMeta | undefined> = calls.map(() => undefined)
  const dispatches: Dispatch[] = []
  for (const [index, call] of calls.entries()) {
    const args = parseArguments(call.input)
    if (args === undefined) {
      results[index] = errorResult(
        call,
        "invalid_arguments",
        "the tool input is not a JSON object"
      )
    } else {
      dispatches.push({ index, call, args })
    }
  }
  const turn: TurnMeta = {
    id: turnId,
    calls: dispatches.length,
    saves: dispatches.filter((d) => isSave(d.call.name)).length,
    usage,
  }
  let last: number | undefined
  let stop: Stop | undefined

  const dispatch = async (
    { index, call, args }: Dispatch,
    admitted: Admitted
  ): Promise<void> => {
    if (admitted.refused) {
      results[index] = {
        callId: call.id,
        content: admitted.content,
        isError: true,
        prunable: false,
      }
      last = index
      return
    }
    const save = isSave(call.name)
    let reply: ToolReply
    try {
      reply = await tools.callTool(
        call.name,
        admitted.args,
        turn,
        toolTimeoutMs,
        signal,
        admitted.hints
      )
    } catch (error) {
      if (signal.aborted) {
        throw error
      }
      watch.failed(call, admitted)
      results[index] = errorResult(
        call,
        "tool_unavailable",
        "the tool call did not complete"
      )
      last = index
      return
    }
    const settled = watch.settle(call, args, admitted, reply)
    stop ??= settled.stop
    metaAt[index] = reply.meta
    results[index] = {
      callId: call.id,
      content: settled.content,
      isError: reply.isError,
      prunable: !save && !reply.isError,
    }
    last = index
  }

  if (turn.calls > 0) {
    watch.beginTurn(turn.saves)
    const reads = dispatches.filter((d) => !isSave(d.call.name))
    const admitted = new Map(
      reads.map((d) => [d.index, watch.admit(d.call, d.args)])
    )
    await inParallel(reads, maxParallelTools, (d) =>
      dispatch(d, admitted.get(d.index) as Admitted)
    )
    for (const saving of dispatches.filter((d) => isSave(d.call.name))) {
      if (stop !== undefined) {
        break
      }
      await dispatch(saving, watch.admit(saving.call, saving.args))
    }
    if (stop === undefined) {
      const end = watch.endTurn()
      stop = end.stop
      const at = last
      const result = at === undefined ? undefined : results[at]
      if (result !== undefined && end.notes.length > 0) {
        result.content = withNotes(result.content, end.notes)
      }
    }
  }
  for (const [index, call] of calls.entries()) {
    results[index] ??= errorResult(
      call,
      "not_run",
      "the run stopped before this call ran"
    )
  }
  return {
    results: results as ToolResult[],
    metas: metaAt.filter((meta) => meta !== undefined),
    stop,
  }
}
