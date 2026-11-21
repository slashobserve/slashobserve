import { useEffect, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "@tanstack/react-router"
import { MoreHorizontalIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { dashboardLink } from "@/lib/links"
import { sentence } from "@/observe/queries"

import { addBlockRowId, useFreezeBlock } from "./add-block-row"
import {
  ArrangeAnnouncer,
  ArrangeHandle,
  DropBar,
  ResizeGrip,
} from "./arrange-handles"
import { PanelBlock } from "./blocks/panel-block"
import { RunBlock } from "./blocks/run-block"
import { SnapshotBlock } from "./blocks/snapshot-block"
import {
  blockHeight,
  blockRows,
  blockTitle,
  hasHeight,
  followsLivePanel,
  holdsAgentOutput,
  nextBlockId,
  rowGridClass,
  spanClass,
  widthSteps,
  type BlockWidth,
} from "./block-rows"
import { blockWriteProblem } from "./errors"
import {
  useMakeDashboard,
  useRemoveBlock,
  type Block,
  type Observation,
} from "./queries"
import { RemoveBlockDialog } from "./remove-block-dialog"
import { RequestedBlock } from "./requested-block"
import { TextBlock } from "./text-block"
import { useArrange, type Arrange } from "./use-arrange"

export type Reveal = { id: string; focus: boolean }

type Props = {
  observation: Observation
  blocks: readonly Block[]
  canAdd: boolean
  reveal: Reveal | null
  onRevealed: () => void
  onAdded: (blockId: string) => void
}

const blocksListId = "observation-blocks"

const widthLabels: Record<BlockWidth, string> = {
  third: "Make one third width",
  half: "Make half width",
  two_thirds: "Make two thirds width",
  full: "Make full width",
}

function sectionId(blockId: string): string {
  return `block-${blockId}`
}

function prefersReducedMotion(): boolean {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches
}

function focusBlock(blockId: string, focus: boolean) {
  const section = document.getElementById(sectionId(blockId))
  if (section === null) return
  section.scrollIntoView({
    block: "start",
    behavior: prefersReducedMotion() ? "auto" : "smooth",
  })
  if (focus) section.focus({ preventScroll: true })
}

function focusAfterRemoval(nextId: string | null) {
  const target =
    (nextId === null ? null : document.getElementById(sectionId(nextId))) ??
    document.getElementById(addBlockRowId) ??
    document.getElementById(blocksListId)
  target?.focus()
}

function BlockMenu({
  observation,
  block,
  arrange,
  canAdd,
  nextId,
  onAdded,
}: {
  observation: Observation
  block: Block
  arrange: Arrange | null
  canAdd: boolean
  nextId: string | null
  onAdded: (blockId: string) => void
}) {
  const remove = useRemoveBlock(observation.id)
  const [confirming, setConfirming] = useState(false)
  const freeze = useFreezeBlock(observation.id, {
    onAdded,
    onProblem: (text) => toast.error(text),
  })
  const canRemove = block.allowed_actions.includes("remove")
  const isLivePanel = block.kind === "panel" && block.state === "live"
  const canFreeze = canAdd && isLivePanel
  const make = useMakeDashboard({
    observationId: observation.id,
    blockId: block.id,
  })
  const navigate = useNavigate()
  if (arrange === null && !canRemove && !isLivePanel) return null

  const position = arrange?.blocks.findIndex((b) => b.id === block.id) ?? -1
  const count = arrange?.blocks.length ?? 0

  function removeBlock() {
    remove.mutate(
      { blockId: block.id, version: block.version },
      {
        onSuccess: () => focusAfterRemoval(nextId),
        onError: (error) => toast.error(blockWriteProblem(error).text),
      }
    )
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Block actions"
              className="absolute -top-3 right-0 z-10 bg-background opacity-0 group-focus-within/block:opacity-100 group-hover/block:opacity-100 focus-visible:opacity-100 aria-expanded:opacity-100"
            />
          }
        >
          <MoreHorizontalIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-48">
          {arrange === null ? null : (
            <>
              {position > 0 ? (
                <DropdownMenuItem onClick={() => arrange.moveBy(block, -1)}>
                  Move up
                </DropdownMenuItem>
              ) : null}
              {position < count - 1 ? (
                <DropdownMenuItem onClick={() => arrange.moveBy(block, 1)}>
                  Move down
                </DropdownMenuItem>
              ) : null}
              {widthSteps
                .filter((width) => width !== block.width)
                .map((width) => (
                  <DropdownMenuItem
                    key={width}
                    onClick={() => arrange.setWidth(block, width)}
                  >
                    {widthLabels[width]}
                  </DropdownMenuItem>
                ))}
              {hasHeight(block) && block.height !== null ? (
                <DropdownMenuItem onClick={() => arrange.resetHeight(block)}>
                  Reset height
                </DropdownMenuItem>
              ) : null}
            </>
          )}
          {canFreeze ? (
            <DropdownMenuItem onClick={() => freeze.freeze(block)}>
              Freeze as snapshot
            </DropdownMenuItem>
          ) : null}
          {isLivePanel ? (
            <DropdownMenuItem
              disabled={make.isPending}
              onClick={() =>
                make.mutate(undefined, {
                  onSuccess: (saved) =>
                    void navigate(
                      dashboardLink(saved.dashboard.id, saved.dashboard.title)
                    ),
                  onError: (error) => toast.error(sentence(error.message)),
                })
              }
            >
              Make a dashboard
            </DropdownMenuItem>
          ) : null}
          {canRemove ? (
            <DropdownMenuItem
              onClick={() =>
                holdsAgentOutput(block) ? setConfirming(true) : removeBlock()
              }
            >
              Delete block
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <RemoveBlockDialog
        title={blockTitle(block)}
        open={confirming}
        onOpenChange={setConfirming}
        onConfirm={removeBlock}
      />
    </>
  )
}

function BlockBody({
  block,
  blocks,
}: {
  block: Block
  blocks: readonly Block[]
}): ReactNode {
  const height = blockHeight(block)
  switch (block.kind) {
    case "text":
      return <TextBlock block={block} />
    case "panel":
      return <PanelBlock block={block} height={height} />
    case "snapshot":
      return <SnapshotBlock block={block} height={height} />
    case "run":
      return (
        <RunBlock
          block={block}
          blocks={blocks}
          onCite={(id) => focusBlock(id, true)}
        />
      )
  }
}

function BlockSection({
  observation,
  block,
  blocks,
  arrange,
  canAdd,
  onAdded,
}: {
  observation: Observation
  block: Block
  blocks: readonly Block[]
  arrange: Arrange | null
  canAdd: boolean
  onAdded: (blockId: string) => void
}) {
  const nextId = nextBlockId(blocks, block.id)
  const body = <BlockBody block={block} blocks={blocks} />
  const dragged = arrange?.dragging?.id === block.id
  return (
    <section
      id={sectionId(block.id)}
      data-block-id={block.id}
      aria-label={blockTitle(block)}
      tabIndex={-1}
      className={`group/block relative min-w-0 rounded-card outline-none focus-visible:ring-2 focus-visible:ring-ring ${spanClass[block.width]} ${dragged ? "opacity-60" : ""}`}
    >
      {arrange === null ? null : (
        <ArrangeHandle block={block} arrange={arrange} />
      )}
      {arrange?.dragging?.drop?.id === block.id ? (
        <DropBar drop={arrange.dragging.drop} />
      ) : null}
      <BlockMenu
        observation={observation}
        block={block}
        arrange={arrange}
        canAdd={canAdd}
        nextId={nextId}
        onAdded={onAdded}
      />
      {block.state === "requested" ? (
        <RequestedBlock
          block={block}
          besideLivePanel={followsLivePanel(blocks, block.id)}
          onKept={() => focusBlock(block.id, true)}
          onDiscarded={() => focusAfterRemoval(nextId)}
        >
          {body}
        </RequestedBlock>
      ) : (
        body
      )}
      {arrange === null ? null : <ResizeGrip block={block} arrange={arrange} />}
    </section>
  )
}

export function BlockList({
  observation,
  blocks,
  canAdd,
  reveal,
  onRevealed,
  onAdded,
}: Props) {
  const listRef = useRef<HTMLOListElement>(null)
  const arranging = useArrange({
    observationId: observation.id,
    blocks,
    listRef,
  })
  const canArrange = observation.allowed_actions.includes("update")
  const arrange = canArrange ? arranging : null
  const shown = arrange === null ? blocks : arrange.blocks

  useEffect(() => {
    if (reveal === null) return
    if (!blocks.some((block) => block.id === reveal.id)) return
    focusBlock(reveal.id, reveal.focus)
    onRevealed()
  }, [reveal, blocks, onRevealed])

  if (blocks.length === 0) {
    return (
      <p
        id={blocksListId}
        tabIndex={-1}
        className="text-[13px] text-muted-foreground outline-none"
      >
        Nothing on this page yet. Add a block, or ask @observe in the
        discussion.
      </p>
    )
  }

  return (
    <>
      <ol
        id={blocksListId}
        ref={listRef}
        aria-label="Blocks"
        tabIndex={-1}
        className="flex flex-col gap-6 outline-none"
      >
        {blockRows(shown).map((row) => (
          <li key={row.key} className="@container">
            <div className={rowGridClass}>
              {row.blocks.map((block) => (
                <BlockSection
                  key={block.id}
                  observation={observation}
                  block={block}
                  blocks={shown}
                  arrange={arrange}
                  canAdd={canAdd}
                  onAdded={onAdded}
                />
              ))}
            </div>
          </li>
        ))}
      </ol>
      {arrange === null ? null : <ArrangeAnnouncer arrange={arrange} />}
    </>
  )
}
