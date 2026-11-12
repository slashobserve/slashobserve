import { type RunTools, type ToolReply } from "./mcp"
import { isSave } from "./phase"
import { type RunMeta, type TurnMeta } from "./protocol"
import type { ToolCall, ToolResult } from "./session/types"

export const maxParallelTools = 4
export const toolTimeoutMs = 300_000

export type Executed = { results: ToolResult[]; metas: RunMeta[] }

type Dispatch = { index: number; call: ToolCall; args: Record<string, unknown> }

function errorResult(call: ToolCall, code: string, message: string) {
  return {
    callId: call.id,
    content: JSON.stringify({ code, message }),
    isError: true,
    prunable: false,
  } satisfies ToolResult
}

function parseArguments(input: string): Record<string, unknown> | undefined {
  try {
    const value: unknown = JSON.parse(input)
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : undefined
  } catch {
    return undefined
  }
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
  signal: AbortSignal
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
  }

  const dispatch = async ({ index, call, args }: Dispatch): Promise<void> => {
    const save = isSave(call.name)
    let reply: ToolReply
    try {
      reply = await tools.callTool(call.name, args, turn, toolTimeoutMs, signal)
    } catch (error) {
      if (signal.aborted) {
        throw error
      }
      results[index] = errorResult(
        call,
        "tool_unavailable",
        "the tool call did not complete"
      )
      return
    }
    metaAt[index] = reply.meta
    results[index] = {
      callId: call.id,
      content: reply.content,
      isError: reply.isError,
      prunable: !save && !reply.isError,
    }
  }

  if (turn.calls > 0) {
    await inParallel(
      dispatches.filter((d) => !isSave(d.call.name)),
      maxParallelTools,
      dispatch
    )
    for (const saving of dispatches.filter((d) => isSave(d.call.name))) {
      await dispatch(saving)
    }
  }
  return {
    results: results as ToolResult[],
    metas: metaAt.filter((meta) => meta !== undefined),
  }
}
