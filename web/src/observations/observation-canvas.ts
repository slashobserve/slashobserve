import { useMemo, useState } from "react"

import type { CanvasHost } from "@/comments/canvas-host"
import type { GateInput } from "@/conversation/delivery"
import { askKeyAction } from "@/dashboards/ask-key"
import {
  openKeyDraft,
  useCanvasTool,
  type ModeTexts,
} from "@/dashboards/canvas-tools"
import { commentKeyAction } from "@/dashboards/comment-key"
import { useAgentInfo } from "@/observe/queries"
import { useHoveredPanel } from "@/shortcuts/hovered-panel"
import { useDashboardShortcuts } from "@/shortcuts/use-dashboard-shortcuts"

import type { Block } from "./queries"

export const observationModeTexts: ModeTexts = {
  comment:
    "Click a spot on a panel block, or select text, to comment on it. Esc returns to Select.",
  ask: "Click a spot on a panel block, or select text, to ask the agent about it. Esc returns to Select.",
}

export type CanvasPlace = {
  canComment: boolean
  arranging: boolean
  editingBlock: string | null
}

export function takesBlockComments(
  block: Pick<Block, "id" | "kind" | "state">,
  place: CanvasPlace
): boolean {
  if (!place.canComment || place.arranging) return false
  if (block.id === place.editingBlock || block.state !== "live") return false
  return block.kind === "panel" || block.kind === "text"
}

export function keyBlockOf(
  blocks: readonly Pick<Block, "id" | "kind" | "state">[],
  candidates: readonly (string | null | undefined)[],
  place: CanvasPlace
): string | undefined {
  for (const id of candidates) {
    const block = blocks.find((each) => each.id === id)
    if (block?.kind === "panel" && takesBlockComments(block, place)) {
      return block.id
    }
  }
  return undefined
}

function focusedBlockId(): string | undefined {
  return document.activeElement?.closest<HTMLElement>("[data-block-id]")
    ?.dataset.blockId
}

type Input = CanvasPlace & {
  observationId: string
  blocks: readonly Block[]
}

export function useObservationCanvas({
  observationId,
  blocks,
  canComment,
  arranging,
  editingBlock,
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
  const place = { canComment, arranging, editingBlock }
  const keyBlock = () =>
    keyBlockOf(
      blocks,
      [useHoveredPanel.getState().target?.id, focusedBlockId()],
      place
    )

  useDashboardShortcuts({
    comment: () => {
      const action = commentKeyAction({
        commenting: tool === "comment",
        panelId: keyBlock(),
        canComment,
        editing: arranging,
        onCanvas: true,
      })
      if (action.kind === "leaveMode")
        useCanvasTool.getState().setTool("select")
      if (action.kind === "enterMode")
        useCanvasTool.getState().setTool("comment")
      if (action.kind === "panel") openKeyDraft(action.panelId, "comment")
    },
    ask: () => {
      const action = askKeyAction({
        asking: tool === "ask",
        panelId: keyBlock(),
        panelAskable: true,
        mayAsk: canComment && agentAvailable === true,
        editing: arranging,
        onCanvas: true,
      })
      if (action.kind === "leaveMode")
        useCanvasTool.getState().setTool("select")
      if (action.kind === "enterMode") useCanvasTool.getState().setTool("ask")
      if (action.kind === "panel") openKeyDraft(action.panelId, "ask")
    },
    close:
      tool === "select"
        ? undefined
        : () => useCanvasTool.getState().setTool("select"),
  })

  return host
}
