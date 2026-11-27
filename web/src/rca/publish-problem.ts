import { ApiError, problemText } from "@/api/errors"
import { blockTitle } from "@/observations/block-rows"
import type { Block } from "@/observations/queries"

const fallback = "Could not publish"

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
  const first = error instanceof ApiError ? error.issues[0]?.message : undefined
  if (
    error instanceof ApiError &&
    error.code === "panel_failed" &&
    first !== undefined
  ) {
    const block = failedBlock(error, blocks)
    return block === undefined
      ? { text: `A panel could not be frozen: ${first}` }
      : {
          text: `The panel "${blockTitle(block)}" could not be frozen: ${first}`,
          blockId: block.id,
        }
  }
  return {
    text: problemText(error, {
      codes: {
        rca_limit: "This observation has 50 revisions, the most it can have.",
        observation_changed:
          "The page changed while it was being published. Publish again.",
        snapshot_too_large:
          "A panel returned too much data to freeze. Narrow its time range and publish again.",
      },
      fallback,
    }),
  }
}
