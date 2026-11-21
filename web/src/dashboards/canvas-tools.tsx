import { useEffect, type ReactNode } from "react"
import {
  AtSignIcon,
  MessageSquarePlusIcon,
  MousePointer2Icon,
} from "lucide-react"
import { create } from "zustand"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { Anchor, Point } from "@/panels/anchors"
import { useAgentInfo } from "@/observe/queries"
import {
  dashboardShortcuts,
  keyLabel,
  type ShortcutAction,
} from "@/shortcuts/chords"

import type { DashboardPermissions } from "./permissions"

export type CanvasTool = "select" | "comment" | "ask"

export type DraftTool = Exclude<CanvasTool, "select">

export type CanvasDraft = {
  panelId: string
  tool: DraftTool
  anchor: Anchor | null
  at: Point | null
  surface: string | null
}

export type OpenTurn = {
  requestId: string
  chatId: string
  panelId: string
  anchor: Anchor | null
  at: Point | null
  surface: string | null
}

type CanvasToolState = {
  tool: CanvasTool
  draft: CanvasDraft | null
  openThreadId: string | null
  openTurn: OpenTurn | null
  postedThreadId: string | null
  text: string
  setTool: (tool: CanvasTool) => void
  openDraft: (draft: CanvasDraft) => void
  openThread: (threadId: string) => void
  showTurn: (turn: OpenTurn) => void
  close: () => void
  posted: (threadId: string) => void
  settled: (threadId: string) => void
  setText: (text: string) => void
}

function holdsText(state: CanvasToolState): boolean {
  const composing = state.draft !== null || state.openTurn !== null
  return composing && state.text.trim() !== ""
}

export const useCanvasTool = create<CanvasToolState>()((set) => ({
  tool: "select",
  draft: null,
  openThreadId: null,
  openTurn: null,
  postedThreadId: null,
  text: "",
  setTool: (tool) =>
    set({ tool, draft: null, openThreadId: null, openTurn: null, text: "" }),
  openDraft: (draft) =>
    set((state) =>
      holdsText(state)
        ? state
        : { draft, openThreadId: null, openTurn: null, text: "" }
    ),
  openThread: (threadId) =>
    set((state) =>
      holdsText(state)
        ? state
        : { openThreadId: threadId, draft: null, openTurn: null, text: "" }
    ),
  showTurn: (turn) =>
    set((state) =>
      holdsText(state)
        ? state
        : { openTurn: turn, draft: null, openThreadId: null, text: "" }
    ),
  close: () =>
    set({ draft: null, openThreadId: null, openTurn: null, text: "" }),
  posted: (threadId) =>
    set({ draft: null, postedThreadId: threadId, text: "" }),
  settled: (threadId) =>
    set((state) =>
      state.postedThreadId === threadId ? { postedThreadId: null } : {}
    ),
  setText: (text) => set({ text }),
}))

export function openKeyDraft(panelId: string, tool: DraftTool) {
  useCanvasTool
    .getState()
    .openDraft({ panelId, tool, anchor: null, at: null, surface: null })
}

const toolShortcuts: Record<CanvasTool, ShortcutAction> = {
  select: "close",
  comment: "comment",
  ask: "ask",
}

function ariaShortcut(keys: readonly string[]): string | undefined {
  if (keys.length !== 1) return undefined
  const [key] = keys
  return key.length === 1 ? key.toUpperCase() : key
}

function toolKeys(tool: CanvasTool): readonly string[] {
  return (
    dashboardShortcuts.find(({ action }) => action === toolShortcuts[tool])
      ?.keys ?? []
  )
}

const toolNames: Record<CanvasTool, string> = {
  select: "Select",
  comment: "Comment",
  ask: "Ask",
}

export const commentModeText =
  "Click a spot on a panel to comment on it. Esc returns to Select."

export const askModeText =
  "Click a spot on a panel to ask the agent about it. Esc returns to Select."

const modeTexts: Record<DraftTool, string> = {
  comment: commentModeText,
  ask: askModeText,
}

export function activeTool(
  tool: CanvasTool,
  may: { comment: boolean; ask: boolean; idle: boolean }
): CanvasTool {
  if (!may.idle || tool === "select") return "select"
  return may[tool] ? tool : "select"
}

type Props = {
  permissions: Pick<DashboardPermissions, "comment" | "edit">
  idle: boolean
}

const toolClass =
  "gap-1.5 text-[13px] aria-pressed:bg-selection aria-pressed:text-foreground"

function ToolButton({
  tool,
  pressed,
  disabled,
  onClick,
  icon,
}: {
  tool: CanvasTool
  pressed?: boolean
  disabled?: boolean
  onClick: () => void
  icon: ReactNode
}) {
  const keys = toolKeys(tool)
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className={toolClass}
            aria-pressed={pressed}
            aria-keyshortcuts={ariaShortcut(keys)}
            disabled={disabled}
            onClick={onClick}
          />
        }
      >
        {icon}
        {toolNames[tool]}
      </TooltipTrigger>
      <TooltipContent>
        {toolNames[tool]}
        <kbd
          aria-hidden="true"
          className="rounded-chip border px-[5px] font-[inherit] text-xs"
        >
          {keys.map(keyLabel).join(" ")}
        </kbd>
      </TooltipContent>
    </Tooltip>
  )
}

export function CanvasTools({ permissions, idle }: Props) {
  const tool = useCanvasTool((state) => state.tool)
  const setTool = useCanvasTool((state) => state.setTool)
  const agentAvailable = useAgentInfo().data?.available === true
  const mayAsk = permissions.comment && agentAvailable
  const active = activeTool(tool, {
    comment: permissions.comment,
    ask: mayAsk,
    idle,
  })

  useEffect(() => () => setTool("select"), [setTool])

  if (!permissions.comment) return null

  return (
    <div className="sticky bottom-0 z-30 h-0 narrow:hidden">
      <div className="absolute bottom-4 left-1/2 flex -translate-x-1/2 flex-col items-center gap-1.5">
        {active === "select" ? null : (
          <p
            role="status"
            className="rounded-control border border-border bg-popover px-2.5 py-1 text-xs whitespace-nowrap text-muted-foreground"
          >
            {modeTexts[active]}
          </p>
        )}
        <div
          role="toolbar"
          aria-label="Canvas tools"
          data-tool={active}
          className="flex h-10 items-center gap-1 rounded-control border border-border bg-popover px-1.5 text-popover-foreground"
        >
          <ToolButton
            tool="select"
            pressed={active === "select"}
            onClick={() => setTool("select")}
            icon={<MousePointer2Icon aria-hidden="true" />}
          />
          <ToolButton
            tool="comment"
            pressed={active === "comment"}
            disabled={!idle}
            onClick={() => setTool("comment")}
            icon={<MessageSquarePlusIcon aria-hidden="true" />}
          />
          {mayAsk ? (
            <ToolButton
              tool="ask"
              pressed={active === "ask"}
              disabled={!idle}
              onClick={() => setTool("ask")}
              icon={<AtSignIcon aria-hidden="true" />}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}
