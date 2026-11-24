import { agentMentionsIn, mentionSegments } from "@/comments/mentions"

export type Delivery = {
  to: "agent" | "everyone"
  visibleTo: "everyone" | "me"
}

export type ThreadAgent = {
  kind: "builtin" | "grant" | "service_account"
  id: string | null
}

export type Place =
  | { kind: "chat"; chatId: string | null }
  | {
      kind: "thread"
      threadId: string | null
      agent: ThreadAgent | null
      agentOff: boolean
    }
  | { kind: "canvas"; tool: "comment" }
  | { kind: "canvas"; tool: "ask"; chatId: string | null }

export type Store = "thread" | "chat"

export function threadPlace(
  threadId: string | null,
  thread?: { agent: ThreadAgent | null; agent_off: boolean } | null
): Place {
  return {
    kind: "thread",
    threadId,
    agent: thread?.agent ?? null,
    agentOff: thread?.agent_off ?? false,
  }
}

function mentionsPerson(body: string): boolean {
  return mentionSegments(body).some(
    (segment) => segment.kind === "mention" && segment.ref.kind === "user"
  )
}

function threadRecipient(
  place: Extract<Place, { kind: "thread" }>,
  body: string
): Delivery["to"] {
  if (agentMentionsIn(body) > 0) return "agent"
  if (mentionsPerson(body)) return "everyone"
  return place.agent !== null && !place.agentOff ? "agent" : "everyone"
}

export function defaultDelivery(place: Place, body: string): Delivery {
  switch (place.kind) {
    case "chat":
      return { to: "agent", visibleTo: "me" }
    case "thread":
      return { to: threadRecipient(place, body), visibleTo: "everyone" }
    case "canvas":
      return place.tool === "ask"
        ? { to: "agent", visibleTo: "me" }
        : {
            to: agentMentionsIn(body) > 0 ? "agent" : "everyone",
            visibleTo: "everyone",
          }
  }
}

export function storeOf(delivery: Delivery): Store | null {
  if (delivery.visibleTo === "everyone") return "thread"
  return delivery.to === "agent" ? "chat" : null
}

export function flips(place: Place): { to: boolean; visibleTo: boolean } {
  switch (place.kind) {
    case "chat":
      return { to: false, visibleTo: false }
    case "thread":
      return { to: true, visibleTo: false }
    case "canvas":
      return place.tool === "ask"
        ? { to: false, visibleTo: false }
        : { to: true, visibleTo: false }
  }
}

export type AgentGate =
  | { kind: "hidden" }
  | { kind: "allowed" }
  | { kind: "no-model" }
  | { kind: "blocked"; reason: string }

export type GateInput = {
  canComment: boolean
  agentAvailable: boolean | undefined
  pinned: boolean
  layoutEditing: boolean
}

export const ungated: GateInput = {
  canComment: true,
  agentAvailable: undefined,
  pinned: false,
  layoutEditing: false,
}

export const pinnedText =
  "You are viewing an older version. Open the latest version to send this to the agent."
export const layoutEditingText =
  "Finish editing the layout to send this to the agent."

export function deliveryGate(input: GateInput, delivery: Delivery): AgentGate {
  if (!input.canComment) return { kind: "hidden" }
  if (delivery.to === "everyone") return { kind: "allowed" }
  if (input.agentAvailable === false) return { kind: "no-model" }
  if (input.pinned) return { kind: "blocked", reason: pinnedText }
  if (input.layoutEditing) return { kind: "blocked", reason: layoutEditingText }
  return { kind: "allowed" }
}

export const noModelReviseText =
  "No model is connected to this server, so the agent cannot make this change."

export function reviseBlock(
  gate: AgentGate,
  builtinAgent: boolean
): string | null {
  switch (gate.kind) {
    case "blocked":
      return gate.reason
    case "no-model":
      return builtinAgent ? noModelReviseText : null
    default:
      return null
  }
}
