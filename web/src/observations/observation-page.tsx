import { useCallback, useEffect, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useThreads } from "@/comments/thread-queries"
import { messageCount } from "@/discussion/discussion-model"
import { useDocumentTitle } from "@/lib/document-title"
import { observationsLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"
import { RcaPanel } from "@/rca/rca-panel"
import { rcasQuery } from "@/rca/queries"

import { AddBlockRow } from "./add-block-row"
import { BlockList, type Reveal } from "./block-list"
import { DiscussionPane } from "./discussion-pane"
import { MovePanel } from "./move-panel"
import { ObservationHeader } from "./observation-header"
import {
  useObservation,
  useObservationLive,
  type ObservationEnvelope,
} from "./queries"

const columnClass = "flex w-full flex-col gap-8"

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
  const [discussionShown, setDiscussionShown] = useState(true)
  const [moving, setMoving] = useState(false)
  const [reveal, setReveal] = useState<Reveal | null>(null)
  const toggleRef = useRef<HTMLButtonElement>(null)
  const headingRef = useRef<HTMLHeadingElement>(null)
  const moreButton = useRef<HTMLButtonElement>(null)
  const focusAfterToggle = useRef<"toggle" | "heading" | null>(null)
  const { threads } = useThreads(`observation:${observation.id}`)
  const count = messageCount(threads)
  useObservationLive(observation)

  useEffect(() => {
    const target = focusAfterToggle.current
    focusAfterToggle.current = null
    if (target === "toggle") toggleRef.current?.focus()
    if (target === "heading") headingRef.current?.focus()
  }, [discussionShown])

  const revealed = useCallback(() => setReveal(null), [])
  const added = useCallback(
    (blockId: string) => setReveal({ id: blockId, focus: true }),
    []
  )
  const onShowBlock = useCallback(
    (blockId: string) => setReveal({ id: blockId, focus: true }),
    []
  )
  const addedFromDiscussion = useCallback(
    (blockId: string) => setReveal({ id: blockId, focus: false }),
    []
  )

  function onToggle() {
    if (discussionShown) {
      setDiscussionShown(false)
      return
    }
    focusAfterToggle.current = "heading"
    setDiscussionShown(true)
  }

  function onClosePane() {
    focusAfterToggle.current = "toggle"
    setDiscussionShown(false)
  }

  return (
    <>
      <div className={columnClass}>
        <ObservationHeader
          envelope={envelope}
          discussion={{
            shown: discussionShown,
            count,
            onToggle,
            toggleRef,
          }}
          onMove={() => setMoving(true)}
          moreButtonRef={moreButton}
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
      {discussionShown ? (
        <DiscussionPane
          observation={observation}
          canAdd={canAdd}
          canEdit={permissions.edit}
          hidden={rca.open || moving}
          headingRef={headingRef}
          onClose={onClosePane}
          onShowBlock={onShowBlock}
          onAdded={addedFromDiscussion}
        />
      ) : null}
      {rca.open ? (
        <RcaPanel
          observation={observation}
          trigger={rcaButton}
          onClose={() => rca.onOpenChange(false)}
        />
      ) : null}
      {moving ? (
        <MovePanel
          observation={observation}
          trigger={moreButton}
          onClose={() => setMoving(false)}
        />
      ) : null}
    </>
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
