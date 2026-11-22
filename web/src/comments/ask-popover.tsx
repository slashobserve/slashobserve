import { useCallback, useEffect, useId, useState, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { useRouteContext, useSearch } from "@tanstack/react-router"
import { LockIcon, XIcon } from "lucide-react"

import { useChat } from "@/chat/chats-queries"
import { ContextChips } from "@/chat/context-chips"
import { ChatAnswer } from "@/chat/request-turn"
import { TurnFailure } from "@/chat/turn-failure"
import { noFindingText } from "@/chat/turn-model"
import { AgentMark } from "@/components/agent-mark"
import { StatusIcon } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Composer } from "@/conversation/composer"
import type { GateInput } from "@/conversation/delivery"
import { arrivingClass } from "@/conversation/thread-view"
import { useComposer, type ComposerModel } from "@/conversation/use-composer"
import { useCanvasTool, useReportSending } from "@/dashboards/canvas-tools"
import {
  useSetDashboardSearch,
  validateDashboardSearch,
} from "@/dashboards/search"
import { cn } from "@/lib/utils"
import {
  isTerminal,
  observeRequestQuery,
  type ObserveRequest,
} from "@/observe/queries"
import type { Anchor, Point } from "@/panels/anchors"

import {
  askChips,
  askWorkingText,
  pinName,
  spotLabel,
  spotTurnIds,
} from "./ask-spot"
import { CanvasPopover, keepsTyping } from "./canvas-composer"
import { isAbout, ownerOf, spotTarget, type CanvasSubject } from "./canvas-host"

export type AskSpot = {
  ownerId: string
  anchor: Anchor | null
  at: Point | null
  surface: string | null
}

type AskedTurn = { requestId: string; chatId: string }

export const privateText = "Only you and Admins can see this."
export const reviseSharedText =
  "A revision is visible to everyone who can see the dashboard."
export const noModelAskText =
  "No model is connected to this server, so the agent cannot answer."

type Props = {
  subject: CanvasSubject
  panelTitle: string
  surfaceRef: RefObject<HTMLElement | null>
  gate: GateInput
  spot: AskSpot
  turn: AskedTurn | null
  openChatId: string | null
  onChatStarted: (chatId: string) => void
}

const newChat = "new"

export function openChatOf(chat: string | undefined): string | null {
  return chat === undefined || chat === newChat ? null : chat
}

type OpenChat = {
  read_only: boolean
  dashboard_id: string | null
  observation_id: string | null
}

function useContinuedChat(
  openChatId: string | null,
  subject: CanvasSubject
): string | null {
  const open = useChat(openChatId ?? undefined).data?.chat
  return continuedChat(openChatId, open, subject)
}

function continuedChat(
  openChatId: string | null,
  open: OpenChat | undefined,
  subject: CanvasSubject
): string | null {
  if (openChatId === null || open === undefined) return null
  return !open.read_only && isAbout(open, subject) ? openChatId : null
}

export function useOpenChat() {
  const chat = useSearch({
    strict: false,
    select: (search: Record<string, unknown>) =>
      validateDashboardSearch(search).chat,
  })
  const setSearch = useSetDashboardSearch()
  const select = useCallback(
    (chatId: string) =>
      setSearch((state) => ({ ...state, chat: chatId }), { replace: true }),
    [setSearch]
  )
  return { chatId: openChatOf(chat), select }
}

function useReturnFocus() {
  const [origin] = useState(() =>
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null
  )
  useEffect(
    () => () => {
      if (focusDropped() && origin !== null && origin.isConnected)
        origin.focus()
    },
    [origin]
  )
}

function focusDropped(): boolean {
  const active = document.activeElement
  return active === null || active === document.body
}

function closeAsk() {
  useCanvasTool.getState().leave("close")
}

function leaveAsk(reason: string) {
  const store = useCanvasTool.getState()
  if (keepsTyping(reason, store.text)) return
  store.leave("close")
}

function useAskText() {
  const value = useCanvasTool((state) => state.text)
  const onChange = useCanvasTool((state) => state.setText)
  return { value, onChange }
}

function disabledReasonOf(model: ComposerModel): string | null {
  return model.gate.kind === "no-model" ? noModelAskText : null
}

function PrivateFooter() {
  return (
    <p className="flex items-center gap-1.5 border-t px-3 py-2 text-xs text-muted-foreground">
      <LockIcon aria-hidden="true" className="size-3.5" />
      {privateText}
    </p>
  )
}

function AskDraft({
  subject,
  panelTitle,
  gate,
  spot,
  openChatId,
  onChatStarted,
}: Omit<Props, "turn" | "surfaceRef">) {
  const [ownDraft] = useState(() => useCanvasTool.getState().draft)
  const chatId = useContinuedChat(openChatId, subject)
  const model = useComposer({
    draft: useAskText(),
    subject,
    place: { kind: "canvas", tool: "ask", chatId },
    target: spotTarget(subject, spot.ownerId, spot.anchor),
    gate,
    onSent: (sent) => {
      if (sent.via !== "chat") return
      if (useCanvasTool.getState().draft !== ownDraft) return
      const asked = sent.request.chat_id ?? sent.request.id
      if (asked !== openChatId) onChatStarted(asked)
      useCanvasTool.getState().showTurn({
        requestId: sent.request.id,
        chatId: asked,
        ...spot,
      })
    },
  })
  useReportSending(model.pending)
  return (
    <div className="p-2">
      <Composer
        model={model}
        layout="docked"
        label="Ask about this"
        formLabel="Ask on the canvas"
        placeholder="Ask about this"
        showVisibility={false}
        disabledReason={disabledReasonOf(model)}
        chips={<ContextChips chips={askChips(panelTitle, spot.anchor)} />}
      />
    </div>
  )
}

function AgentPart({
  request,
  subject,
  arrived,
  onAsked,
}: {
  request: ObserveRequest
  subject: CanvasSubject
  arrived: boolean
  onAsked: (request: ObserveRequest) => void
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  if (request.status === "queued" || request.status === "running") {
    return (
      <p
        role="status"
        className="flex items-center gap-2 text-[13px] text-muted-foreground"
      >
        <StatusIcon tone="running" />
        {askWorkingText(request)}
      </p>
    )
  }
  if (request.status === "failed") {
    return <TurnFailure request={request} onRetried={onAsked} />
  }
  if (request.finding == null) {
    return (
      <p className="text-[13px] text-muted-foreground">
        {noFindingText(request)}
      </p>
    )
  }
  return (
    <div className={cn(arrived && arrivingClass)}>
      <ChatAnswer
        request={request}
        finding={request.finding}
        own={request.requested_by.id === principal.id}
        about={subject}
        onRevised={async (next) => onAsked(next)}
        showToEveryone={false}
        reviseNote={reviseSharedText}
      />
    </div>
  )
}

function AskTurn({
  requestId,
  subject,
  onAsked,
}: {
  requestId: string
  subject: CanvasSubject
  onAsked: (request: ObserveRequest) => void
}) {
  const query = useQuery(observeRequestQuery(requestId))
  const [settledAtFirst] = useState(() => isTerminal(query.data))
  const request = query.data
  if (request === undefined) {
    return (
      <li className="text-[13px] text-muted-foreground">
        {query.isError ? "This question could not be loaded." : "Loading"}
      </li>
    )
  }
  return (
    <li className="flex flex-col gap-2.5" data-request={requestId}>
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium">You</span>
        <p className="text-[13px] wrap-anywhere whitespace-pre-wrap">
          {request.prompt}
        </p>
      </div>
      <div className="flex gap-2.5">
        <AgentMark variant="tile" className="size-6 rounded-chip" />
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <AgentPart
            request={request}
            subject={subject}
            arrived={!settledAtFirst}
            onAsked={onAsked}
          />
        </div>
      </div>
    </li>
  )
}

function AskThread({
  subject,
  panelTitle,
  gate,
  spot,
  turn,
}: Omit<Props, "openChatId" | "onChatStarted" | "surfaceRef"> & {
  turn: AskedTurn
}) {
  const chat = useChat(turn.chatId)
  const [askedHere, setAskedHere] = useState<string[]>([])
  const placed = (chat.data?.turns ?? []).map((each) => ({
    ...each,
    panel_id: ownerOf(subject, each),
  }))
  const ids = spotTurnIds(
    placed,
    { requestId: turn.requestId, panelId: spot.ownerId, anchor: spot.anchor },
    askedHere
  )
  function asked(request: ObserveRequest) {
    setAskedHere((current) =>
      current.includes(request.id) ? current : [...current, request.id]
    )
  }
  const model = useComposer({
    draft: useAskText(),
    subject,
    place: { kind: "canvas", tool: "ask", chatId: turn.chatId },
    target: spotTarget(subject, spot.ownerId, spot.anchor),
    gate,
    onSent: (sent) => {
      if (sent.via === "chat") asked(sent.request)
    },
  })
  useReportSending(model.pending)
  const field = model.field
  useEffect(() => {
    if (focusDropped()) field.current?.focus()
  }, [field])
  return (
    <>
      <div className="flex items-center gap-0.5 border-b py-1.5 pr-1.5 pl-3">
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {spotLabel(panelTitle, spot.anchor)}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Close"
          onClick={closeAsk}
        >
          <XIcon aria-hidden="true" />
        </Button>
      </div>
      <ol
        aria-label="Your chat on this spot"
        className="flex max-h-[min(28rem,60vh)] min-h-0 [scrollbar-width:thin] [scrollbar-color:var(--muted-foreground)_transparent] flex-col gap-3.5 overflow-y-auto p-3"
      >
        {ids.map((id) => (
          <AskTurn key={id} requestId={id} subject={subject} onAsked={asked} />
        ))}
      </ol>
      <div className="px-3 pb-2.5">
        <Composer
          model={model}
          layout="docked"
          label="Ask a follow-up"
          formLabel="Ask a follow-up"
          placeholder="Ask a follow-up"
          showVisibility={false}
          disabledReason={disabledReasonOf(model)}
        />
      </div>
    </>
  )
}

export function AskPopover(props: Props) {
  useReturnFocus()
  const popupId = useId()
  const { turn, spot, panelTitle } = props
  return (
    <CanvasPopover
      id={popupId}
      surfaceRef={props.surfaceRef}
      at={spot.at}
      label={
        turn === null
          ? `Ask about ${spotLabel(panelTitle, spot.anchor)}`
          : pinName(panelTitle, spot.anchor)
      }
      initialFocus={() =>
        document.getElementById(popupId)?.querySelector("textarea") ?? true
      }
      className="flex flex-col border-primary"
      onClose={leaveAsk}
    >
      {turn === null ? (
        <AskDraft {...props} />
      ) : (
        <AskThread {...props} turn={turn} />
      )}
      <PrivateFooter />
    </CanvasPopover>
  )
}
