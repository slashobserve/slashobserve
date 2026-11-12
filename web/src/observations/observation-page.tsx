import { useCallback, useEffect, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { Message } from "@/comments/thread-queries"
import { Discussion } from "@/discussion/discussion"
import { useDocumentTitle } from "@/lib/document-title"
import { observationsLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"
import { RcaPanel } from "@/rca/rca-panel"
import { rcasQuery } from "@/rca/queries"

import { AddBlockRow } from "./add-block-row"
import { addProblemText } from "./block-rows"
import { BlockList, type Reveal } from "./block-list"
import { ObservationHeader } from "./observation-header"
import {
  useAddBlock,
  useObservation,
  useObservationLive,
  type ObservationEnvelope,
} from "./queries"

const columnClass = "mx-auto flex w-full max-w-[720px] flex-col gap-8"

function ObservationSkeleton() {
  return (
    <div aria-busy="true" className={columnClass}>
      <Skeleton className="h-7 w-1/2" />
      <div className="flex flex-col gap-6">
        {[0, 1, 2].map((row) => (
          <Skeleton key={row} className="h-40 rounded-card" />
        ))}
      </div>
    </div>
  )
}

function ObservationNotFound() {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Observation not found</h1>
      <p className="text-sm text-muted-foreground">
        This observation does not exist or you cannot see it
      </p>
      <Link
        {...observationsLink()}
        className={cn(buttonVariants({ variant: "outline" }))}
      >
        Back to Observations
      </Link>
    </div>
  )
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
    <div>
      <Button
        variant="ghost"
        size="sm"
        aria-disabled={addBlock.isPending || undefined}
        aria-busy={addBlock.isPending || undefined}
        onClick={add}
      >
        {canEdit ? "Add to page" : "Suggest for page"}
      </Button>
    </div>
  )
}

type RcaControl = { open: boolean; onOpenChange: (open: boolean) => void }

function LoadedObservation({
  envelope,
  rca,
}: {
  envelope: ObservationEnvelope
  rca: RcaControl
}) {
  const { observation, permissions } = envelope
  const blocks = observation.blocks ?? []
  const canAdd = permissions.edit || permissions.comment
  const canPublish = observation.allowed_actions.includes("publish")
  const revisions = useQuery(rcasQuery(observation.id))
  const hasRevisions = (revisions.data?.length ?? 0) > 0
  const rcaButton = useRef<HTMLButtonElement>(null)
  const [discussionHidden, setDiscussionHidden] = useState(false)
  const [reveal, setReveal] = useState<Reveal | null>(null)
  const showButton = useRef<HTMLButtonElement>(null)
  const discussionBox = useRef<HTMLDivElement>(null)
  const focusAfterToggle = useRef<"show" | "discussion" | null>(null)
  useObservationLive(observation)

  useEffect(() => {
    const target = focusAfterToggle.current
    focusAfterToggle.current = null
    if (target === "show") showButton.current?.focus()
    if (target === "discussion") discussionBox.current?.focus()
  }, [discussionHidden])

  const revealed = useCallback(() => setReveal(null), [])
  const added = useCallback(
    (blockId: string) => setReveal({ id: blockId, focus: true }),
    []
  )
  const addedFromDiscussion = useCallback(
    (blockId: string) => setReveal({ id: blockId, focus: false }),
    []
  )

  return (
    <div className="@container">
      <div className="flex flex-col gap-6 @min-[1100px]:flex-row @min-[1100px]:items-start">
        <div className="min-w-0 flex-1">
          <div className={columnClass}>
            <ObservationHeader
              envelope={envelope}
              discussionHidden={discussionHidden}
              showDiscussionRef={showButton}
              onShowDiscussion={() => {
                focusAfterToggle.current = "discussion"
                setDiscussionHidden(false)
              }}
              extraActions={
                canPublish || hasRevisions ? (
                  <Button
                    ref={rcaButton}
                    variant="outline"
                    size="sm"
                    onClick={() => rca.onOpenChange(true)}
                  >
                    {canPublish ? "Publish as RCA" : "RCA revisions"}
                  </Button>
                ) : null
              }
            />
            <BlockList
              observation={observation}
              blocks={blocks}
              canAdd={canAdd}
              reveal={reveal}
              onRevealed={revealed}
              onAdded={added}
            />
            {canAdd ? (
              <AddBlockRow
                observation={observation}
                blocks={blocks}
                canEdit={permissions.edit}
                onAdded={added}
              />
            ) : null}
          </div>
        </div>
        {discussionHidden ? null : (
          <div
            ref={discussionBox}
            tabIndex={-1}
            className="w-full outline-none @min-[1100px]:sticky @min-[1100px]:top-3.5 @min-[1100px]:h-[calc(100svh-4.75rem)] @min-[1100px]:w-96 @min-[1100px]:flex-none"
          >
            <Discussion
              className="h-full max-h-[480px] @min-[1100px]:max-h-none"
              target={`observation:${observation.id}`}
              hint="Messages stay here until someone adds them to the page"
              emptyText="No messages yet. Ask a question or type @observe to start a run."
              onHide={() => {
                focusAfterToggle.current = "show"
                setDiscussionHidden(true)
              }}
              messageActions={
                canAdd
                  ? (message) => (
                      <AddToPage
                        observationId={observation.id}
                        message={message}
                        canEdit={permissions.edit}
                        onAdded={addedFromDiscussion}
                      />
                    )
                  : undefined
              }
            />
          </div>
        )}
      </div>
      {rca.open ? (
        <RcaPanel
          observation={observation}
          trigger={rcaButton}
          onClose={() => rca.onOpenChange(false)}
        />
      ) : null}
    </div>
  )
}

export function ObservationPage({
  observationId,
  rca,
}: {
  observationId: string
  rca: RcaControl
}) {
  const query = useObservation(observationId)
  const title = query.data?.observation.title
  useDocumentTitle(
    title === undefined
      ? "Observation - SlashObserve"
      : `${title} - Observations - SlashObserve`
  )
  if (query.data !== undefined)
    return <LoadedObservation envelope={query.data} rca={rca} />
  if (query.error instanceof ApiError && query.error.status === 404) {
    return <ObservationNotFound />
  }
  if (query.isError) {
    return (
      <div className={columnClass}>
        <LoadProblem
          title="Could not load this observation"
          detail={sentence(query.error.message)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </div>
    )
  }
  return <ObservationSkeleton />
}
