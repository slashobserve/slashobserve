export type RequestOrigin = "observe" | "prompt" | "comment" | "investigate"

export type OriginFields = {
  kind?: "build" | "investigate" | undefined
  intent: "apply" | "ask"
  base_version: number | null
  comment_id: string | null
}

export function requestOrigin(request: OriginFields): RequestOrigin {
  if (request.kind === "investigate") return "investigate"
  if (request.intent === "ask" || request.base_version === null) {
    return "observe"
  }
  return request.comment_id === null ? "prompt" : "comment"
}
