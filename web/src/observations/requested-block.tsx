import { useState, type ReactNode } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { observationKeys } from "@/api/keys"
import { Button } from "@/components/ui/button"

import { blockWriteProblem } from "./errors"
import { useDiscardBlock, useKeepBlock, type Block } from "./queries"

type Props = {
  block: Block
  children: ReactNode
  onKept: () => void
  onDiscarded: () => void
}

type Pressed = "keep" | "discard"

export function RequestedBlock({
  block,
  children,
  onKept,
  onDiscarded,
}: Props) {
  const queryClient = useQueryClient()
  const keep = useKeepBlock(block.observation_id)
  const discard = useDiscardBlock(block.observation_id)
  const [pressed, setPressed] = useState<Pressed | null>(null)
  const canKeep = block.allowed_actions.includes("keep")
  const canDiscard = block.allowed_actions.includes("discard")
  const pending = keep.isPending || discard.isPending

  function fail(error: unknown) {
    const problem = blockWriteProblem(error)
    toast.error(problem.text)
    if (problem.kind === "refused") {
      void queryClient.invalidateQueries({
        queryKey: observationKeys.one(block.observation_id),
        exact: true,
      })
    }
  }

  function keepIt() {
    if (pending) return
    setPressed("keep")
    keep.mutate(
      { blockId: block.id, version: block.version },
      { onSuccess: onKept, onError: fail }
    )
  }

  function discardIt() {
    if (pending) return
    setPressed("discard")
    discard.mutate(
      { blockId: block.id },
      { onSuccess: onDiscarded, onError: fail }
    )
  }

  const caption =
    block.created_by.run_id === null
      ? `${block.created_by.label} suggests this block`
      : "Observe Agent suggests this block"

  return (
    <div className="flex flex-col gap-3 rounded-card border border-dashed border-border p-3">
      <p className="text-xs text-muted-foreground">{caption}</p>
      {children}
      {canKeep || canDiscard ? (
        <div className="flex items-center gap-2">
          {canKeep ? (
            <Button
              size="sm"
              aria-disabled={pending || undefined}
              aria-busy={pending && pressed === "keep" ? true : undefined}
              onClick={keepIt}
            >
              Keep
            </Button>
          ) : null}
          {canDiscard ? (
            <Button
              variant="outline"
              size="sm"
              aria-disabled={pending || undefined}
              aria-busy={pending && pressed === "discard" ? true : undefined}
              onClick={discardIt}
            >
              Discard
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
