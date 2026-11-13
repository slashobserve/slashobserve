import { useContext, useId, useMemo, type RefObject } from "react"
import { createPortal } from "react-dom"
import { toast } from "sonner"

import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { EntryAction } from "@/activity/entry-frame"
import type { Message } from "@/comments/thread-queries"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { DiscussionFeed } from "@/discussion/discussion"
import { PanelHostContext } from "@/shell/side-panel"

import { addProblemText } from "./block-rows"
import { observationChanges } from "./discussion-changes"
import { useAddBlock, type Observation } from "./queries"

type Props = {
  observation: Observation
  canAdd: boolean
  canEdit: boolean
  hidden: boolean
  headingRef: RefObject<HTMLHeadingElement | null>
  onClose: () => void
  onShowBlock: (blockId: string) => void
  onAdded: (blockId: string) => void
}

function AddToPage({
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

  function add() {
    if (addBlock.isPending) return
    addBlock.mutate(
      { kind: "text", body: { markdown: message.body } },
      {
        onSuccess: ({ block }) => {
          toast.success(
            canEdit ? "Added to the page" : "Suggested for the page"
          )
          onAdded(block.id)
        },
        onError: (error) => toast.error(addProblemText(error)),
      }
    )
  }

  return (
    <EntryAction
      icon={null}
      aria-disabled={addBlock.isPending || undefined}
      aria-busy={addBlock.isPending || undefined}
      onClick={add}
    >
      {canEdit ? "Add to page" : "Suggest for page"}
    </EntryAction>
  )
}

export function DiscussionPane({
  observation,
  canAdd,
  canEdit,
  hidden,
  headingRef,
  onClose,
  onShowBlock,
  onAdded,
}: Props) {
  const host = useContext(PanelHostContext)
  const layout = useActivityLayout(false)
  const setWidth = useActivityStore((state) => state.setWidth)
  const headingId = useId()
  const entries = useMemo(() => observationChanges(observation), [observation])

  if (!host) return null
  return createPortal(
    <DockPanel
      labelledBy={headingId}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={setWidth}
      stacked="container"
      hidden={hidden}
    >
      <DockHeader
        headingId={headingId}
        headingRef={headingRef}
        title="Discussion"
        description="Messages, agent answers and changes to the page."
        onClose={onClose}
        closeLabel="Close panel"
      />
      <DiscussionFeed
        target={`observation:${observation.id}`}
        hint="Messages stay here until someone adds them to the page"
        emptyText="No messages yet. Ask a question or leave a note."
        changes={{ entries, onSelect: onShowBlock }}
        messageActions={
          canAdd
            ? (message) => (
                <AddToPage
                  observationId={observation.id}
                  message={message}
                  canEdit={canEdit}
                  onAdded={onAdded}
                />
              )
            : undefined
        }
      />
    </DockPanel>,
    host
  )
}
