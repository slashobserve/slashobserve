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
  | { kind: "canvas"; tool: "comment" | "ask" }

export type Store = "thread" | "chat"

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
        : { to: "everyone", visibleTo: "everyone" }
  }
}

export function storeOf(delivery: Delivery): Store | null {
  if (delivery.visibleTo === "everyone") return "thread"
  return delivery.to === "agent" ? "chat" : null
}
