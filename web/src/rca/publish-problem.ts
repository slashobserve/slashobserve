import { ApiError } from "@/api/errors"
import { blockTitle } from "@/observations/block-rows"
import type { Block } from "@/observations/queries"
import { sentence } from "@/observe/queries"

const fallback = "Could not publish"

function serverSentence(error: ApiError): string {
  return error.message.trim() === "" ? fallback : sentence(error.message)
}

export type PublishProblem = { text: string; blockId?: string }

function failedBlock(error: ApiError, blocks: readonly Block[]) {
  const path = error.issues[0]?.path ?? ""
  const match = /^\/blocks\/(\d+)$/.exec(path)
  if (match === null) return undefined
  return blocks.find((block) => block.position === Number(match[1]))
}

export function publishProblem(
  error: unknown,
  blocks: readonly Block[] = []
): PublishProblem {
  return describe(error, blocks)
}

function describe(error: unknown, blocks: readonly Block[]): PublishProblem {
  if (!(error instanceof ApiError)) return { text: fallback }
  switch (error.code) {
    case "rca_limit":
      return {
        text: "This observation has 50 revisions, the most it can have.",
      }
    case "observation_changed":
      return {
        text: "The page changed while it was being published. Publish again.",
      }
    case "panel_failed": {
      const first = error.issues[0]?.message
      if (first === undefined) return { text: serverSentence(error) }
      const block = failedBlock(error, blocks)
      return block === undefined
        ? { text: `A panel could not be frozen: ${first}` }
        : {
            text: `The panel "${blockTitle(block)}" could not be frozen: ${first}`,
            blockId: block.id,
          }
    }
    case "snapshot_too_large":
      return {
        text: "A panel returned too much data to freeze. Narrow its time range and publish again.",
      }
    default:
      return { text: serverSentence(error) }
  }
}
