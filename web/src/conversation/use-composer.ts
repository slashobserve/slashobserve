import { useRef, useState, type RefObject } from "react"
import { useRouteContext } from "@tanstack/react-router"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import {
  freshCursor,
  stepHistory,
  usePromptHistory,
  type HistoryCursor,
} from "@/chat/prompt-history"
import { emptyPromptText } from "@/chat/prompt-length"
import {
  chatCommands,
  commandLabels,
  takeCommand,
  withCommand,
} from "@/chat/slash-commands"
import {
  agentMentionsIn,
  mentionedAgents,
  type MentionAgent,
  type MentionPerson,
} from "@/comments/mentions"
import {
  askedAgent,
  useOpenThread,
  useApplySuggestedChange,
  useReplyToThread,
  type SentMessage,
} from "@/comments/thread-queries"
import {
  useInlineMention,
  type InlineMention,
} from "@/comments/use-inline-mention"
import {
  useMentionPicks,
  type MentionPicks,
} from "@/comments/use-mention-picks"
import { useMentionableAgents } from "@/comments/use-mentionable-agents"
import { roleAtLeast } from "@/lib/roles"
import { agentZone } from "@/lib/time"
import { cleanText } from "@/lib/user-text"
import {
  describeSubmitError,
  sentence,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { maxPromptLength } from "@/prompts/rules"

import {
  defaultDelivery,
  deliveryGate,
  flips,
  type AgentGate,
  type Delivery,
  type GateInput,
  type Place,
} from "./delivery"
import {
  planSend,
  threadTargetOf,
  type Opens,
  type PageCommand,
  type Send,
  type SpotTarget,
  type Subject,
} from "./send-plan"

export type SentResult =
  | { via: "thread"; sent: SentMessage }
  | { via: "chat"; request: ObserveRequest; opens: Opens }

export type ComposerInput = {
  subject: Subject | null
  place: Place
  target?: SpotTarget
  gate: GateInput
  onSent: (sent: SentResult) => void | Promise<void>
  draft?: { value: string; onChange: (text: string) => void }
  initialText?: string
  people?: readonly MentionPerson[]
  replyToId?: string
  range?: { from: string; to: string }
  timeZone?: string
  runningRequestId?: string
}

type Failure = { text: string; invalid: boolean }

export type ComposerModel = {
  text: string
  wire: string
  setText: (value: string, caret?: number) => void
  replaceText: (value: string) => void
  delivery: Delivery
  flips: { to: boolean; visibleTo: boolean }
  recipient: string
  turnTo: () => void
  turnVisibleTo: () => void
  gate: AgentGate
  commands: readonly PageCommand[]
  command: PageCommand | null
  clearCommand: () => void
  mention: InlineMention
  picks: MentionPicks
  agents: readonly MentionAgent[]
  people: readonly MentionPerson[]
  mentionTarget: string | null
  failure: Failure | null
  started: { sent: SentMessage; agentName: string } | null
  announcement: { text: string; count: number }
  pending: boolean
  sendBlock: string | null
  runningRequestId: string | undefined
  field: RefObject<HTMLTextAreaElement | null>
  submit: () => Promise<void>
  recall: (direction: "older" | "newer") => boolean
  browsingHistory: boolean
}

export const everyoneHere = "everyone here"
export const oneAgentText = "Mention one agent per message"
const openFailedText = "Sent, but it could not be opened."

function newClientId(): string {
  return crypto.randomUUID()
}

export function recipientName(
  place: Place,
  body: string,
  agents: readonly MentionAgent[]
): string {
  const named = mentionedAgents(body)[0]
  if (named !== undefined) return named.name
  if (place.kind === "thread" && place.agent !== null) {
    if (place.agent.kind === "builtin") return "Observe Agent"
    const id = place.agent.id?.toLowerCase()
    const found = agents.find(
      (agent) =>
        agent.kind === place.agent?.kind && agent.id.toLowerCase() === id
    )
    return found?.name ?? "the thread's agent"
  }
  return "Observe Agent"
}

export function chosenDelivery(
  place: Place,
  body: string,
  chosen: Partial<Delivery>
): Delivery {
  const fallback = defaultDelivery(place, body)
  const can = flips(place)
  return {
    to: can.to ? (chosen.to ?? fallback.to) : fallback.to,
    visibleTo: can.visibleTo
      ? (chosen.visibleTo ?? fallback.visibleTo)
      : fallback.visibleTo,
  }
}

export function placeKey(place: Place): string {
  switch (place.kind) {
    case "chat":
      return `chat:${place.chatId ?? ""}`
    case "thread":
      return `thread:${place.threadId ?? ""}`
    case "canvas":
      return `canvas:${place.tool}`
  }
}

export function sendBlockOf(
  body: string,
  delivery: Delivery,
  gate: AgentGate
): string | null {
  if (agentMentionsIn(body) > 1) return oneAgentText
  if (delivery.to === "agent" && gate.kind === "blocked") return gate.reason
  return null
}

function threadFailureText(error: unknown): Failure {
  if (error instanceof ApiError) {
    if (error.status === 422 && error.code === "unknown_panel") {
      return {
        text: "That panel no longer exists. Pick another target.",
        invalid: true,
      }
    }
    return error.status >= 500
      ? {
          text: "The message could not be sent right now. Try again.",
          invalid: false,
        }
      : { text: sentence(error.message), invalid: false }
  }
  return {
    text: "The message could not be sent. Check your connection and try again.",
    invalid: false,
  }
}

function chatFailureText(error: unknown): Failure | null {
  const described = describeSubmitError(error)
  if (!described.inline) {
    toast.error(described.message)
    return null
  }
  return { text: described.message, invalid: described.invalid }
}

export function useDeliver(subject: Subject | null, timeZone?: string) {
  const zone = timeZone ?? agentZone(undefined)
  const target = subject === null ? "" : threadTargetOf(subject)
  const openThread = useOpenThread(target)
  const replyToThread = useReplyToThread(target)
  const applyChange = useApplySuggestedChange(target)
  const createRequest = useCreateObserveRequest()

  async function deliver(send: Send): Promise<SentResult> {
    switch (send.via) {
      case "thread.open":
        return {
          via: "thread",
          sent: await openThread.mutateAsync({
            body: send.body,
            to: send.to,
            clientId: send.clientId,
            panelId: send.panelId,
            blockId: send.blockId,
            anchor: send.anchor,
            timeZone: zone,
          }),
        }
      case "thread.reply":
        return {
          via: "thread",
          sent: await replyToThread.mutateAsync({
            threadId: send.threadId,
            body: send.body,
            to: send.to,
            clientId: send.clientId,
            blockId: send.blockId,
            replyToId: send.replyToId,
            timeZone: zone,
          }),
        }
      case "thread.apply":
        return {
          via: "thread",
          sent: await applyChange.mutateAsync({
            threadId: send.threadId,
            findingId: send.findingId,
          }),
        }
      case "chat.turn":
        return {
          via: "chat",
          request: await createRequest.mutateAsync({
            ...send.input,
            ...(timeZone === undefined ? {} : { timeZone }),
          }),
          opens: send.opens,
        }
    }
  }

  return {
    deliver,
    pending:
      openThread.isPending ||
      replyToThread.isPending ||
      applyChange.isPending ||
      createRequest.isPending,
  }
}

export function useComposer(input: ComposerInput): ComposerModel {
  const { principal } = useRouteContext({ from: "/_authed" })
  const { subject, place } = input
  const [localText, setLocalText] = useState(input.initialText ?? "")
  const text = input.draft?.value ?? localText
  const writeText = input.draft?.onChange ?? setLocalText
  const [command, setCommand] = useState<PageCommand | null>(null)
  const [chosen, setChosen] = useState<Partial<Delivery>>({})
  const [clientId, setClientId] = useState(newClientId)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [started, setStarted] = useState<ComposerModel["started"]>(null)
  const [cursor, setCursor] = useState<HistoryCursor>(freshCursor)
  const [announcement, setAnnouncement] = useState({ text: "", count: 0 })
  const [sending, setSending] = useState(false)
  const inFlight = useRef(false)
  const key = placeKey(place)
  const [seenKey, setSeenKey] = useState(key)
  if (seenKey !== key) {
    setSeenKey(key)
    setChosen({})
  }
  const field = useRef<HTMLTextAreaElement | null>(null)
  const mention = useInlineMention()
  const picks = useMentionPicks()
  const history = usePromptHistory()
  const agents = useMentionableAgents(place.kind !== "chat")
  const mentionTarget = subject === null ? null : threadTargetOf(subject)
  const delivering = useDeliver(subject, input.timeZone)
  const { deliver } = delivering

  const commands =
    place.kind === "chat"
      ? chatCommands(roleAtLeast(principal.role, "editor"))
      : []
  const wire = picks.wire(cleanText(text))
  const delivery = chosenDelivery(place, wire, chosen)
  const gate = deliveryGate(input.gate, delivery)
  const sendBlock = sendBlockOf(wire, delivery, gate)
  const pending = sending || delivering.pending

  function announce(next: string) {
    setAnnouncement((current) => ({ text: next, count: current.count + 1 }))
  }

  function setText(value: string, caret?: number) {
    const taken = takeCommand(value, commands)
    if (taken !== null) {
      setCommand(taken.command)
      announce(`${commandLabels[taken.command]} on`)
      writeText(taken.rest)
    } else {
      if (caret !== undefined) mention.typed(text, value, caret)
      writeText(value)
    }
    setCursor(freshCursor)
    setFailure(null)
  }

  function replaceText(value: string) {
    writeText(value)
    setFailure(null)
  }

  function clearCommand() {
    setCommand(null)
    announce("")
  }

  function turnTo() {
    setChosen((current) => ({
      ...current,
      to: delivery.to === "agent" ? "everyone" : "agent",
    }))
  }

  function turnVisibleTo() {
    setChosen((current) => ({
      ...current,
      visibleTo: delivery.visibleTo === "me" ? "everyone" : "me",
    }))
  }

  function recall(direction: "older" | "newer"): boolean {
    const step = stepHistory(
      history.entries,
      cursor,
      direction,
      withCommand(command, text)
    )
    if (step === null) return false
    setCursor(step.cursor)
    setCommand(null)
    const restored = picks.adopt(step.text)
    const taken = takeCommand(restored, commands)
    if (taken !== null) setCommand(taken.command)
    writeText(taken?.rest ?? restored)
    setFailure(null)
    requestAnimationFrame(() => {
      const element = field.current
      if (element === null) return
      const at = direction === "older" ? 0 : element.value.length
      element.setSelectionRange(at, at)
    })
    return true
  }

  function returnFocus() {
    const active = document.activeElement
    const elsewhere =
      active !== null &&
      active !== document.body &&
      !field.current?.form?.contains(active)
    if (!elsewhere) field.current?.focus()
  }

  function fail(next: Failure | null) {
    setFailure(next)
    returnFocus()
  }

  function sent() {
    if (delivery.to === "agent") history.record(withCommand(command, wire))
    writeText("")
    setCommand(null)
    setChosen({})
    setCursor(freshCursor)
    setClientId(newClientId())
    returnFocus()
  }

  async function submit() {
    if (pending || inFlight.current) return
    if (wire === "") {
      if (place.kind === "chat") {
        fail({
          text: emptyPromptText(
            subject?.kind === "dashboard" ? { dashboardId: subject.id } : {}
          ),
          invalid: true,
        })
      }
      return
    }
    if ([...wire].length > maxPromptLength) {
      fail({
        text: `Use at most ${maxPromptLength} characters.`,
        invalid: true,
      })
      return
    }
    if (sendBlock !== null) {
      fail({ text: sendBlock, invalid: false })
      return
    }
    const plan = planSend(place, subject, delivery, {
      body: wire,
      clientId,
      command,
      target: input.target,
      replyToId: input.replyToId,
      range: input.range,
      toChosen: chosen.to !== undefined,
    })
    if ("error" in plan) {
      fail({ text: plan.error, invalid: true })
      return
    }
    setFailure(null)
    setStarted(null)
    inFlight.current = true
    setSending(true)
    const agentName = recipientName(place, wire, agents)
    let result: SentResult
    try {
      result = await deliver(plan)
    } catch (error) {
      inFlight.current = false
      setSending(false)
      fail(
        plan.via === "chat.turn"
          ? chatFailureText(error)
          : threadFailureText(error)
      )
      return
    }
    sent()
    if (result.via === "thread") {
      if (askedAgent(result.sent)) setStarted({ sent: result.sent, agentName })
      announce(
        result.sent.to === "agent" ? "Sent to the agent" : "Comment added"
      )
    } else {
      announce("Sent to the agent")
    }
    try {
      await input.onSent(result)
    } catch {
      toast.error(openFailedText)
    } finally {
      inFlight.current = false
      setSending(false)
    }
  }

  return {
    text,
    wire,
    setText,
    replaceText,
    delivery,
    flips: flips(place),
    recipient:
      delivery.to === "agent"
        ? recipientName(place, wire, agents)
        : everyoneHere,
    turnTo,
    turnVisibleTo,
    gate,
    commands,
    command,
    clearCommand,
    mention,
    picks,
    agents,
    people: input.people ?? [],
    mentionTarget,
    failure,
    started,
    announcement,
    pending,
    sendBlock,
    runningRequestId: input.runningRequestId,
    field,
    submit,
    recall,
    browsingHistory: cursor.index !== -1,
  }
}
