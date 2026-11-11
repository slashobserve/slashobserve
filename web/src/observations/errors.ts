import { ApiError, toApiError, unwrap } from "@/api/errors"
import type { components } from "@/api/schema.gen"
import { sentence } from "@/observe/queries"

type Block = components["schemas"]["ObservationBlock"]

export type BlockWriteProblem =
  | { kind: "conflict"; current: Block; text: string }
  | { kind: "refused"; text: string }
  | { kind: "invalid"; text: string }
  | { kind: "other"; text: string }

export class BlockVersionConflict extends ApiError {
  readonly current: Block

  constructor(error: ApiError, current: Block) {
    super(error.status, error.code, error.message)
    this.name = "BlockVersionConflict"
    this.current = current
  }
}

const conflictText =
  "Someone changed this block. The latest is shown; your change was not saved."
const otherText = "The change could not be saved. Try again."

function currentBlock(body: unknown): Block | undefined {
  if (typeof body !== "object" || body === null) return undefined
  const current = (body as { current?: unknown }).current
  if (typeof current !== "object" || current === null) return undefined
  return typeof (current as { id?: unknown }).id === "string"
    ? (current as Block)
    : undefined
}

export function unwrapBlockWrite<T>(result: {
  data?: T
  error?: unknown
  response: Response
}): T {
  if (result.response.status === 409 && result.error !== undefined) {
    const error = toApiError(result.response, result.error)
    const current = currentBlock(result.error)
    if (error.code === "version_conflict" && current !== undefined) {
      throw new BlockVersionConflict(error, current)
    }
  }
  return unwrap(result)
}

export function blockWriteProblem(error: unknown): BlockWriteProblem {
  if (error instanceof BlockVersionConflict) {
    return { kind: "conflict", current: error.current, text: conflictText }
  }
  if (error instanceof ApiError) {
    if (error.code === "invalid_transition") {
      return { kind: "refused", text: sentence(error.message) }
    }
    if (error.status === 422) {
      return { kind: "invalid", text: sentence(error.message) }
    }
  }
  return { kind: "other", text: otherText }
}
