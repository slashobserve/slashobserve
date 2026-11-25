import { useEffect, useState, type ReactNode } from "react"
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
import { cn } from "@/lib/utils"
import type { Anchor, Point } from "@/panels/anchors"
import { useAgentInfo } from "@/observe/queries"
import {
  dashboardShortcuts,
  keyLabel,
  type ShortcutAction,
} from "@/shortcuts/chords"

import { HeldDraft } from "./held-draft"
import { useSlotBox } from "./use-slot-box"

import type { DashboardPermissions } from "./permissions"

export type CanvasTool = "select" | "comment" | "ask"

export type DraftTool = Exclude<CanvasTool, "select">

export type CanvasDraft = {
  ownerId: string
  tool: DraftTool
  anchor: Anchor | null
  at: Point | null
  surface: string | null
}

export type OpenTurn = {
  requestId: string
  chatId: string
  ownerId: string
  anchor: Anchor | null
  at: Point | null
  surface: string | null
}

export type Leaving = CanvasTool | "close"

export type DetachedDraft = { tool: DraftTool; text: string }

type CanvasToolState = {
  tool: CanvasTool
  draft: CanvasDraft | null
  openThreadId: string | null
  openTurn: OpenTurn | null
  postedThreadId: string | null
  text: string
  sending: boolean
  leaving: Leaving | null
  detached: DetachedDraft | null
  setTool: (tool: CanvasTool) => void
  leave: (to: Leaving) => void
  keepTyping: () => void
  discard: () => void
  detach: () => void
  dropDetached: () => void
  openDraft: (draft: CanvasDraft) => void
  openThread: (threadId: string) => void
  showTurn: (turn: OpenTurn) => void
  close: () => void
  posted: (threadId: string) => void
  settled: (threadId: string) => void
  setText: (text: string) => void
  setSending: (sending: boolean) => void
}

function holdsText(state: CanvasToolState): boolean {
  const composing = state.draft !== null || state.openTurn !== null
  return composing && !state.sending && state.text.trim() !== ""
}

const closed = {
  draft: null,
  openThreadId: null,
  openTurn: null,
  text: "",
  sending: false,
}

function moved(to: Leaving) {
  return to === "close" ? closed : { ...closed, tool: to }
}

export const useCanvasTool = create<CanvasToolState>()((set) => ({
  tool: "select",
  draft: null,
  openThreadId: null,
  openTurn: null,
  postedThreadId: null,
  text: "",
  sending: false,
  leaving: null,
  detached: null,
  setTool: (tool) => set({ ...closed, tool, leaving: null }),
  leave: (to) =>
    set((state) =>
      holdsText(state) ? { leaving: to } : { ...moved(to), leaving: null }
    ),
  keepTyping: () => set({ leaving: null }),
  discard: () =>
    set((state) =>
      state.leaving === null
        ? state
        : { ...moved(state.leaving), leaving: null }
    ),
  detach: () =>
    set((state) => {
      if (state.draft === null && state.openTurn === null) return state
      const detached = holdsText(state)
        ? { tool: state.draft?.tool ?? "ask", text: state.text }
        : state.detached
      return { ...closed, detached, leaving: null }
    }),
  dropDetached: () => set({ detached: null }),
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
  close: () => set(closed),
  posted: (threadId) =>
    set({ draft: null, postedThreadId: threadId, text: "", sending: false }),
  settled: (threadId) =>
    set((state) =>
      state.postedThreadId === threadId ? { postedThreadId: null } : {}
    ),
  setText: (text) => set({ text }),
  setSending: (sending) => set({ sending }),
}))

export function useReportSending(pending: boolean) {
  useEffect(() => {
    useCanvasTool.getState().setSending(pending)
    return () => useCanvasTool.getState().setSending(false)
  }, [pending])
}

export function openKeyDraft(ownerId: string, tool: DraftTool) {
  useCanvasTool
    .getState()
    .openDraft({ ownerId, tool, anchor: null, at: null, surface: null })
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

export type ModeTexts = Record<DraftTool, string>

const dashboardModeTexts: ModeTexts = {
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
  modeTexts?: ModeTexts
  pageFooter?: boolean
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

export function CanvasTools({
  permissions,
  idle,
  modeTexts = dashboardModeTexts,
  pageFooter = false,
}: Props) {
  const tool = useCanvasTool((state) => state.tool)
  const setTool = useCanvasTool((state) => state.setTool)
  const leave = useCanvasTool((state) => state.leave)
  const agentAvailable = useAgentInfo().data?.available === true
  const mayAsk = permissions.comment && agentAvailable
  const active = activeTool(tool, {
    comment: permissions.comment,
    ask: mayAsk,
    idle,
  })

  useEffect(() => () => setTool("select"), [setTool])

  return (
    <>
      <HeldDraft />
      {permissions.comment ? (
        <ToolBar
          active={active}
          idle={idle}
          mayAsk={mayAsk}
          modeTexts={modeTexts}
          pageFooter={pageFooter}
          leave={leave}
        />
      ) : null}
    </>
  )
}

function ToolBar({
  active,
  idle,
  mayAsk,
  modeTexts,
  pageFooter,
  leave,
}: {
  active: CanvasTool
  idle: boolean
  mayAsk: boolean
  modeTexts: ModeTexts
  pageFooter: boolean
  leave: (to: Leaving) => void
}) {
  const bar = (
    <div
      role="toolbar"
      aria-label="Canvas tools"
      data-tool={active}
      className="pointer-events-auto flex h-10 items-center gap-1 rounded-control border border-border bg-popover px-1.5 text-popover-foreground"
    >
      <ToolButton
        tool="select"
        pressed={active === "select"}
        onClick={() => leave("select")}
        icon={<MousePointer2Icon aria-hidden="true" />}
      />
      <ToolButton
        tool="comment"
        pressed={active === "comment"}
        disabled={!idle}
        onClick={() => leave("comment")}
        icon={<MessageSquarePlusIcon aria-hidden="true" />}
      />
      {mayAsk ? (
        <ToolButton
          tool="ask"
          pressed={active === "ask"}
          disabled={!idle}
          onClick={() => leave("ask")}
          icon={<AtSignIcon aria-hidden="true" />}
        />
      ) : null}
    </div>
  )
  const docked = active !== "select"
  const hint = docked ? (
    <p role="status" className="text-center text-xs text-muted-foreground">
      {modeTexts[active]}
    </p>
  ) : null
  if (pageFooter) {
    return (
      <PageFooter docked={docked} bar={bar}>
        {hint}
      </PageFooter>
    )
  }
  return (
    <div
      className={cn(
        "sticky bottom-0 z-30 narrow:hidden",
        docked
          ? "flex flex-none flex-wrap items-center justify-center gap-x-3 gap-y-1 border-t bg-background px-4 py-2"
          : "pointer-events-none h-0"
      )}
    >
      <div
        className={cn(
          docked ? "contents" : "absolute bottom-4 left-1/2 -translate-x-1/2"
        )}
      >
        {bar}
      </div>
      {hint}
    </div>
  )
}

const floatingBottomInset = 16

function PageFooter({
  docked,
  bar,
  children,
}: {
  docked: boolean
  bar: ReactNode
  children: ReactNode
}) {
  const [slot, setSlot] = useState<HTMLDivElement | null>(null)
  const bottom = docked ? 0 : floatingBottomInset
  const { left, width, clipped } = useSlotBox(slot, bottom)
  return (
    <div ref={setSlot} data-page-footer="" className="-mt-8 h-24 narrow:hidden">
      <div
        style={{ left, width, bottom }}
        className={cn(
          "fixed z-30 flex flex-wrap items-center justify-center gap-x-3 gap-y-1",
          docked ? "border-t bg-background px-4 py-2" : "pointer-events-none",
          clipped && "invisible"
        )}
      >
        {bar}
        {children}
      </div>
    </div>
  )
}
