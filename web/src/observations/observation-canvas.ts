import { useEffect, useMemo, useState } from "react"

import type { CanvasHost } from "@/comments/canvas-host"
import type { GateInput } from "@/conversation/delivery"
import { askKeyAction } from "@/dashboards/ask-key"
import {
  openKeyDraft,
  useCanvasTool,
  type ModeTexts,
} from "@/dashboards/canvas-tools"
import { commentKeyAction } from "@/dashboards/comment-key"
import { conversationActionsFor } from "@/dashboards/panel-actions"
import { useAgentInfo } from "@/observe/queries"
import { useHoveredPanel } from "@/shortcuts/hovered-panel"
import { useDashboardShortcuts } from "@/shortcuts/use-dashboard-shortcuts"
import { isRowPanel } from "@/spec"

import { panelBlockBody } from "./blocks/block-data"
import type { Block } from "./queries"

export const observationModeTexts: ModeTexts = {
  comment:
    "Click a spot on a block, or select text, to comment on it. Esc returns to Select.",
  ask: "Click a spot on a block, or select text, to ask the agent about it. Esc returns to Select.",
}

type Placed = Pick<Block, "id" | "kind" | "state">

export function takesBlockComments(
  block: Pick<Block, "state">,
  canComment: boolean
): boolean {
  return canComment && block.state === "live"
}

export function keyBlockOf<T extends Placed>(
  blocks: readonly T[],
  candidates: readonly (string | null | undefined)[],
  takes: (block: T) => boolean
): T | undefined {
  for (const id of candidates) {
    const block = blocks.find((each) => each.id === id)
    if (block !== undefined && takes(block)) return block
  }
  return undefined
}

export function blockAskable(
  block: Block | undefined,
  canComment: boolean,
  agentAvailable: boolean
): boolean {
  if (block === undefined) return false
  if (block.kind !== "panel") return canComment && agentAvailable
  const panel = panelBlockBody(block)?.panel
  if (panel === undefined || isRowPanel(panel)) return false
  return conversationActionsFor(panel, canComment, agentAvailable).ask
}

export function ownerGone(
  blocks: readonly Pick<Block, "id" | "state">[],
  ownerId: string,
  canComment: boolean
): boolean {
  const block = blocks.find((each) => each.id === ownerId)
  return block === undefined || !takesBlockComments(block, canComment)
}

function focusedBlockId(): string | undefined {
  return document.activeElement?.closest<HTMLElement>("[data-block-id]")
    ?.dataset.blockId
}

function surfaceShown(blockId: string): boolean {
  return (
    document.querySelector(
      `[data-block-id="${CSS.escape(blockId)}"] [data-canvas-surface]`
    ) !== null
  )
}

type Input = {
  observationId: string
  blocks: readonly Block[]
  canComment: boolean
  arranging: boolean
}

export function useObservationCanvas({
  observationId,
  blocks,
  canComment,
  arranging,
}: Input): CanvasHost {
  const agentAvailable = useAgentInfo().data?.available
  const [chatId, setChatId] = useState<string | null>(null)
  const gate = useMemo<GateInput>(
    () => ({
      canComment,
      agentAvailable,
      pinned: false,
      layoutEditing: arranging,
    }),
    [canComment, agentAvailable, arranging]
  )
  const host = useMemo<CanvasHost>(
    () => ({
      subject: { kind: "observation", id: observationId },
      gate,
      chat: { chatId, select: setChatId },
    }),
    [observationId, gate, chatId]
  )
  const tool = useCanvasTool((state) => state.tool)
  const composing = useCanvasTool(
    (state) => state.draft !== null || state.openTurn !== null
  )
  const owner = useCanvasTool(
    (state) => state.draft?.ownerId ?? state.openTurn?.ownerId ?? null
  )

  useEffect(() => {
    if (owner !== null && ownerGone(blocks, owner, canComment)) {
      useCanvasTool.getState().detach()
    }
  }, [owner, blocks, canComment])

  const keyBlock = () =>
    keyBlockOf(
      blocks,
      [useHoveredPanel.getState().target?.id, focusedBlockId()],
      (block) => takesBlockComments(block, canComment) && surfaceShown(block.id)
    )
  const leave = useCanvasTool.getState().leave

  useDashboardShortcuts({
    comment: () => {
      const action = commentKeyAction({
        commenting: tool === "comment",
        panelId: keyBlock()?.id,
        canComment,
        editing: arranging,
        onCanvas: true,
      })
      if (action.kind === "leaveMode") leave("select")
      if (action.kind === "enterMode") leave("comment")
      if (action.kind === "panel") openKeyDraft(action.panelId, "comment")
    },
    ask: () => {
      const block = keyBlock()
      const action = askKeyAction({
        asking: tool === "ask",
        panelId: block?.id,
        panelAskable: blockAskable(block, canComment, agentAvailable === true),
        mayAsk: canComment && agentAvailable === true,
        editing: arranging,
        onCanvas: true,
      })
      if (action.kind === "leaveMode") leave("select")
      if (action.kind === "enterMode") leave("ask")
      if (action.kind === "panel") openKeyDraft(action.panelId, "ask")
    },
    ...(tool !== "select" || composing ? { close: () => leave("select") } : {}),
  })

  return host
}
