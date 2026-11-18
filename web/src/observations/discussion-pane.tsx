import { useContext, useId, useMemo, useState, type RefObject } from "react"
import { createPortal } from "react-dom"
import { useQueries, useQuery } from "@tanstack/react-query"
import { toast } from "sonner"

import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { EntryAction } from "@/activity/entry-frame"
import { ShowFilter } from "@/activity/show-filter"
import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { observationKeys } from "@/api/keys"
import type { Message } from "@/comments/thread-queries"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { useDatasources } from "@/datasources/queries"
import { BlockSelect, DiscussionFeed } from "@/discussion/discussion"
import { noFilter, type DiscussionFilter } from "@/discussion/discussion-filter"
import type { DiscussionShow } from "@/discussion/discussion-model"
import { PanelHostContext } from "@/shell/side-panel"

import { snapshotIdOf, snapshotQuery } from "./blocks/block-data"
import { addProblemText } from "./block-rows"
import {
  blockSubject,
  changeEventTypes,
  observationChanges,
} from "./discussion-changes"
import { useAddBlock, type Block, type Observation } from "./queries"

const showOptions: { value: DiscussionShow; label: string }[] = [
  { value: "everything", label: "Everything" },
  { value: "messages", label: "Messages only" },
  { value: "changes", label: "Changes only" },
]

const changeEventLimit = 100

type Props = {
  observation: Observation
  folderTitle: string
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

type FrozenPanel = { id: string; panel: { [key: string]: unknown } }

function snapshotTitlesOf(
  results: readonly { data?: FrozenPanel }[]
): ReadonlyMap<string, string> {
  const titles = new Map<string, string>()
  for (const { data } of results) {
    const title = data?.panel.title
    if (
      data !== undefined &&
      typeof title === "string" &&
      title.trim() !== ""
    ) {
      titles.set(data.id, title)
    }
  }
  return titles
}

function useSnapshotTitles(blocks: readonly Block[]) {
  const ids = blocks.flatMap((block) => snapshotIdOf(block) ?? [])
  return useQueries({
    queries: ids.map(snapshotQuery),
    combine: snapshotTitlesOf,
  })
}

function useChangeEvents(observation: Observation) {
  return useQuery({
    queryKey: observationKeys.changes(observation.id, observation.updated_at),
    queryFn: ({ signal }) =>
      api
        .GET("/activity", {
          params: {
            query: {
              observation_id: observation.id,
              type: [...changeEventTypes],
              limit: changeEventLimit,
            },
          },
          signal,
        })
        .then(unwrap)
        .then((body) => body.activity),
    placeholderData: (previous) => previous,
    staleTime: Infinity,
  })
}

export function DiscussionPane({
  observation,
  folderTitle,
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
  const datasources = useDatasources()
  const events = useChangeEvents(observation)
  const snapshotTitles = useSnapshotTitles(observation.blocks ?? [])
  const entries = useMemo(
    () =>
      observationChanges(observation, {
        folderTitle,
        datasources: datasources.data ?? [],
        events: events.data ?? [],
        snapshotTitles,
      }),
    [observation, folderTitle, datasources.data, events.data, snapshotTitles]
  )
  const blocks = useMemo(
    () =>
      (observation.blocks ?? [])
        .toSorted((a, b) => a.position - b.position)
        .map((block) => ({
          id: block.id,
          title: blockSubject(block, snapshotTitles),
        })),
    [observation.blocks, snapshotTitles]
  )
  const [show, setShow] = useState<DiscussionShow>("everything")
  const [filterBlock, setFilterBlock] = useState<string | null>(null)
  const [anchor, setAnchor] = useState<string | null>(null)
  const blockFilterId = useId()
  const filter: DiscussionFilter = {
    show,
    blockId: blocks.some((block) => block.id === filterBlock)
      ? filterBlock
      : null,
  }

  function chooseFilterBlock(blockId: string | null) {
    setFilterBlock(blockId)
    if (blockId !== null) setAnchor(blockId)
  }

  function clearFilter() {
    setShow(noFilter.show)
    setFilterBlock(noFilter.blockId)
  }

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
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5 border-b px-4 py-2">
        <ShowFilter options={showOptions} value={show} onChange={setShow} />
        <label
          htmlFor={blockFilterId}
          className="text-xs text-muted-foreground"
        >
          Block
        </label>
        <BlockSelect
          id={blockFilterId}
          label="Block"
          noneLabel="All blocks"
          blocks={blocks}
          value={filter.blockId}
          onChange={chooseFilterBlock}
          className="max-w-44 min-w-0"
        />
      </div>
      <DiscussionFeed
        target={`observation:${observation.id}`}
        hint="Messages stay here until someone adds them to the page"
        emptyText="No messages yet. Ask a question or leave a note."
        changes={{ entries, onSelect: onShowBlock }}
        blocks={{
          items: blocks,
          anchor,
          onAnchorChange: setAnchor,
          onShow: onShowBlock,
        }}
        view={{ filter, onClear: clearFilter }}
        messageActions={(message, { reply }) =>
          reply === null && !canAdd ? undefined : (
            <>
              {reply === null ? null : (
                <EntryAction
                  icon={null}
                  aria-label={`Reply to ${message.actor.label}`}
                  onClick={reply}
                >
                  Reply
                </EntryAction>
              )}
              {canAdd ? (
                <AddToPage
                  observationId={observation.id}
                  message={message}
                  canEdit={canEdit}
                  onAdded={onAdded}
                />
              ) : null}
            </>
          )
        }
      />
    </DockPanel>,
    host
  )
}
