import { useEffect, type ReactNode } from "react"
import { MoreHorizontalIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import { addBlockRowId, useFreezeBlock } from "./add-block-row"
import { PanelBlock } from "./blocks/panel-block"
import { RunBlock } from "./blocks/run-block"
import { SnapshotBlock } from "./blocks/snapshot-block"
import { blockRows, blockTitle, nextBlockId } from "./block-rows"
import { blockWriteProblem } from "./errors"
import {
  useRemoveBlock,
  useUpdateBlock,
  type Block,
  type Observation,
} from "./queries"
import { RequestedBlock } from "./requested-block"
import { TextBlock } from "./text-block"

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
const halfHeight = 220
const fullHeight = 280

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
  canAdd,
  nextId,
  onAdded,
}: {
  observation: Observation
  block: Block
  canAdd: boolean
  nextId: string | null
  onAdded: (blockId: string) => void
}) {
  const update = useUpdateBlock(observation.id)
  const remove = useRemoveBlock(observation.id)
  const freeze = useFreezeBlock(observation.id, {
    onAdded,
    onProblem: (text) => toast.error(text),
  })
  const canUpdate = block.allowed_actions.includes("update")
  const canRemove = block.allowed_actions.includes("remove")
  const canFreeze = canAdd && block.kind === "panel" && block.state === "live"
  if (!canUpdate && !canRemove && !canFreeze) return null

  const half = block.width === "half"

  return (
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
        {canUpdate ? (
          <DropdownMenuItem
            onClick={() =>
              update.mutate(
                {
                  blockId: block.id,
                  version: block.version,
                  width: half ? "full" : "half",
                },
                {
                  onError: (error) =>
                    toast.error(blockWriteProblem(error).text),
                }
              )
            }
          >
            {half ? "Make full width" : "Make half width"}
          </DropdownMenuItem>
        ) : null}
        {canFreeze ? (
          <DropdownMenuItem onClick={() => freeze.freeze(block)}>
            Freeze as snapshot
          </DropdownMenuItem>
        ) : null}
        {canRemove ? (
          <DropdownMenuItem
            onClick={() =>
              remove.mutate(
                { blockId: block.id, version: block.version },
                {
                  onSuccess: () => focusAfterRemoval(nextId),
                  onError: (error) =>
                    toast.error(blockWriteProblem(error).text),
                }
              )
            }
          >
            Delete block
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function BlockBody({ block }: { block: Block }): ReactNode {
  const height = block.width === "half" ? halfHeight : fullHeight
  switch (block.kind) {
    case "text":
      return <TextBlock block={block} />
    case "panel":
      return <PanelBlock block={block} height={height} />
    case "snapshot":
      return <SnapshotBlock block={block} height={height} />
    case "run":
      return <RunBlock block={block} onCite={(id) => focusBlock(id, true)} />
  }
}

function BlockSection({
  observation,
  block,
  blocks,
  canAdd,
  onAdded,
}: {
  observation: Observation
  block: Block
  blocks: readonly Block[]
  canAdd: boolean
  onAdded: (blockId: string) => void
}) {
  const nextId = nextBlockId(blocks, block.id)
  const body = <BlockBody block={block} />
  return (
    <section
      id={sectionId(block.id)}
      aria-label={blockTitle(block)}
      tabIndex={-1}
      className="group/block relative min-w-0 rounded-card outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <BlockMenu
        observation={observation}
        block={block}
        canAdd={canAdd}
        nextId={nextId}
        onAdded={onAdded}
      />
      {block.state === "requested" ? (
        <RequestedBlock
          block={block}
          onKept={() => focusBlock(block.id, true)}
          onDiscarded={() => focusAfterRemoval(nextId)}
        >
          {body}
        </RequestedBlock>
      ) : (
        body
      )}
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
    <ol
      id={blocksListId}
      aria-label="Blocks"
      tabIndex={-1}
      className="flex flex-col gap-6 outline-none"
    >
      {blockRows(blocks).map((row) => (
        <li key={row.key} className="@container">
          <div
            className={
              row.blocks[0].width === "half"
                ? "grid grid-cols-1 gap-4 @min-[720px]:grid-cols-2"
                : "grid grid-cols-1 gap-4"
            }
          >
            {row.blocks.map((block) => (
              <BlockSection
                key={block.id}
                observation={observation}
                block={block}
                blocks={blocks}
                canAdd={canAdd}
                onAdded={onAdded}
              />
            ))}
          </div>
        </li>
      ))}
    </ol>
  )
}
