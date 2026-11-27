import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { opensByDefault } from "@/components/dock/card-width"
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
import {
  BlockEditorContext,
  BlockEditorDock,
  type BlockEditRequest,
  type ReplaceGuard,
} from "./block-editor"
import { BlockList, type Reveal } from "./block-list"
import {
  BlockInspectContext,
  BlockInspectDock,
  useBlockInspectState,
} from "./blocks/block-inspect"
import { PageRangeContext } from "./blocks/block-data"
import { DiscussionPane } from "./discussion-pane"
import { observationIsMissing } from "./errors"
import { MovePanel } from "./move-panel"
import { ObservationHeader } from "./observation-header"
import {
  useObservation,
  useObservationLive,
  type ObservationEnvelope,
} from "./queries"
import { plainTitle } from "@/comments/plain-title"

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
  focusedThreadId,
  onClearThread,
}: {
  envelope: ObservationEnvelope
  rca: RcaControl
  focusedThreadId?: string
  onClearThread: () => void
}) {
  const { observation, permissions } = envelope
  const blocks = observation.blocks ?? []
  const canAdd = permissions.edit || permissions.comment
  const canPublish = observation.allowed_actions.includes("publish")
  const isPrivate = envelope.folder.personal
  const mustMoveToPublish =
    isPrivate && observation.allowed_actions.includes("move")
  const revisions = useQuery(rcasQuery(observation.id))
  const hasRevisions = (revisions.data?.length ?? 0) > 0
  const rcaButton = useRef<HTMLButtonElement>(null)
  const [discussionShown, setDiscussionShown] = useState(
    () => focusedThreadId !== undefined || opensByDefault(window.innerWidth)
  )
  const [openedFor, setOpenedFor] = useState(focusedThreadId)
  if (openedFor !== focusedThreadId) {
    setOpenedFor(focusedThreadId)
    if (focusedThreadId !== undefined) setDiscussionShown(true)
  }
  const [moving, setMoving] = useState(false)
  const [picking, setPicking] = useState(false)
  const [editing, setEditing] = useState<BlockEditRequest | null>(null)
  const [editorKey, setEditorKey] = useState(0)
  const replaceGuard = useRef<ReplaceGuard | null>(null)
  const [reveal, setReveal] = useState<Reveal | null>(null)
  const [footerSlot, setFooterSlot] = useState<HTMLDivElement | null>(null)
  const toggleRef = useRef<HTMLButtonElement>(null)
  const headingRef = useRef<HTMLHeadingElement>(null)
  const moreButton = useRef<HTMLButtonElement>(null)
  const focusAfterToggle = useRef<"toggle" | "heading" | null>(null)
  const { threads } = useThreads(`observation:${observation.id}`)
  const count = messageCount(threads)
  useObservationLive(observation)
  const inspect = useBlockInspectState(blocks)
  const pageRange = useMemo(
    () => ({ from: observation.from, to: observation.to }),
    [observation.from, observation.to]
  )
  const inspecting = inspect.block !== undefined
  const replaceEditor = useCallback((replace: () => void) => {
    if (replaceGuard.current === null) replace()
    else replaceGuard.current(replace)
  }, [])
  const blockEditor = useMemo(
    () => ({
      open: (request: BlockEditRequest) => {
        if (
          request.kind === "edit" &&
          editing?.kind === "edit" &&
          editing.blockId === request.blockId
        ) {
          return
        }
        replaceEditor(() => {
          setPicking(false)
          setEditing(request)
          setEditorKey((key) => key + 1)
        })
      },
      canSuggest: permissions.comment,
    }),
    [permissions.comment, editing, replaceEditor]
  )
  const closeEditor = useCallback(() => setEditing(null), [])
  const savedFromEditor = useCallback((blockId: string) => {
    setEditing(null)
    setReveal({ id: blockId, focus: true })
  }, [])

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
      if (focusedThreadId !== undefined) onClearThread()
      return
    }
    focusAfterToggle.current = "heading"
    setDiscussionShown(true)
  }

  function onClosePane() {
    focusAfterToggle.current = "toggle"
    setDiscussionShown(false)
    if (focusedThreadId !== undefined) onClearThread()
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
            canPublish || mustMoveToPublish || hasRevisions ? (
              <Button
                ref={rcaButton}
                variant="outline"
                size="sm"
                onClick={() => rca.onOpenChange(true)}
              >
                {canPublish || mustMoveToPublish
                  ? "Publish as RCA"
                  : "RCA revisions"}
              </Button>
            ) : null
          }
        />
        <BlockEditorContext value={canAdd ? blockEditor : null}>
          <PageRangeContext value={pageRange}>
            <BlockInspectContext value={inspect.context}>
              <BlockList
                observation={observation}
                blocks={blocks}
                canAdd={canAdd}
                canComment={permissions.comment}
                reveal={reveal}
                onRevealed={revealed}
                onAdded={added}
                footerSlot={footerSlot}
              />
            </BlockInspectContext>
          </PageRangeContext>
          {canAdd ? (
            <AddBlockRow
              observation={observation}
              blocks={blocks}
              canEdit={permissions.edit}
              picking={picking}
              onPickingChange={(next) => {
                if (!next) setPicking(false)
                else {
                  replaceEditor(() => {
                    setEditing(null)
                    setPicking(true)
                  })
                }
              }}
              onAdded={added}
            />
          ) : null}
        </BlockEditorContext>
        <div ref={setFooterSlot} className="contents" />
      </div>
      {discussionShown ? (
        <DiscussionPane
          observation={observation}
          folderTitle={envelope.folder.title}
          canAdd={canAdd}
          canEdit={permissions.edit}
          hidden={
            rca.open || moving || picking || inspecting || editing !== null
          }
          headingRef={headingRef}
          focusedThreadId={focusedThreadId}
          onClose={onClosePane}
          onShowBlock={onShowBlock}
          onAdded={addedFromDiscussion}
        />
      ) : null}
      {editing !== null ? (
        <PageRangeContext value={pageRange}>
          <BlockEditorDock
            key={editorKey}
            hidden={rca.open || moving}
            observation={observation}
            blocks={blocks}
            request={editing}
            canEdit={permissions.edit}
            onClose={closeEditor}
            onSaved={savedFromEditor}
            replaceGuardRef={replaceGuard}
          />
        </PageRangeContext>
      ) : null}
      {inspect.block !== undefined &&
      !(rca.open || moving || picking || editing !== null) ? (
        <BlockInspectDock
          block={inspect.block}
          tab={inspect.tab}
          target={inspect.target}
          onTabChange={inspect.setTab}
          onClose={inspect.close}
        />
      ) : null}
      {rca.open ? (
        <RcaPanel
          observation={observation}
          trigger={rcaButton}
          blocks={blocks}
          onShowBlock={onShowBlock}
          onClose={() => rca.onOpenChange(false)}
          privateFolder={
            mustMoveToPublish
              ? () => {
                  rca.onOpenChange(false)
                  setMoving(true)
                }
              : undefined
          }
        />
      ) : null}
      {moving ? (
        <MovePanel
          observation={observation}
          confined={envelope.confined}
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
  focusedThreadId,
  onClearThread,
}: {
  observationId: string
  rca: RcaControl
  focusedThreadId?: string
  onClearThread: () => void
}) {
  const query = useObservation(observationId)
  const title =
    query.data === undefined
      ? undefined
      : plainTitle(query.data.observation.title)
  useDocumentTitle(
    title === undefined
      ? "Observation - SlashObserve"
      : `${title} - Observations - SlashObserve`
  )
  if (query.data !== undefined)
    return (
      <LoadedObservation
        envelope={query.data}
        rca={rca}
        focusedThreadId={focusedThreadId}
        onClearThread={onClearThread}
      />
    )
  if (observationIsMissing(query.error)) {
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
