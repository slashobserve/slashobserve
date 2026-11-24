import { toast } from "sonner"

import { EntryAction } from "@/activity/entry-frame"
import type { Message } from "@/comments/thread-queries"

import { addProblemText } from "./block-rows"
import { useAddBlock, type AddBlockInput } from "./queries"

export function blocksOfMessage(message: Message): AddBlockInput[] | null {
  const answer = message.answer ?? null
  if (answer === null) {
    if (message.actor.run_id !== null) return null
    return [{ kind: "text", body: { markdown: message.body } }]
  }
  const { title, markdown, evidence, from, to } = answer.finding
  return [
    { kind: "text", body: { markdown: `## ${title}\n\n${markdown}` } },
    ...evidence.map((panel): AddBlockInput => ({
      kind: "panel",
      body: { panel, from, to },
    })),
  ]
}

export function AddToPage({
  observationId,
  message,
  canEdit,
  onAdded,
}: {
  observationId: string
  message: Message
  canEdit: boolean
  onAdded: (blockId: string) => void
}) {
  const addBlock = useAddBlock(observationId)
  const blocks = blocksOfMessage(message)
  if (blocks === null) return null

  async function add(inputs: AddBlockInput[]) {
    if (addBlock.isPending) return
    try {
      const [first, ...rest] = inputs
      const { block } = await addBlock.mutateAsync(first)
      toast.success(canEdit ? "Added to the page" : "Suggested for the page")
      onAdded(block.id)
      for (const input of rest) await addBlock.mutateAsync(input)
    } catch (error) {
      toast.error(addProblemText(error))
    }
  }

  return (
    <EntryAction
      icon={null}
      aria-disabled={addBlock.isPending || undefined}
      aria-busy={addBlock.isPending || undefined}
      onClick={() => void add(blocks)}
    >
      {canEdit ? "Add to page" : "Suggest for page"}
    </EntryAction>
  )
}
