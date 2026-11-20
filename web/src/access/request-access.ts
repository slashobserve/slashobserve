import { useMutation } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import type { components } from "@/api/schema.gen"
import { formatShortRelative } from "@/lib/time"

export type AccessObject = {
  kind: components["schemas"]["AccessRequest"]["object"]["kind"]
  id: string
}

export const maxNoteLength = 200

export type RequestOutcome = "sent" | "open" | "can_comment" | "failed"

export function noteProblem(note: string): string | null {
  if (/[\r\n]/.test(note)) return "Keep the note to one line."
  if ([...note.trim()].length > maxNoteLength) {
    return `Keep the note to ${maxNoteLength} characters.`
  }
  return null
}

export function outcomeOf(error: unknown): RequestOutcome {
  if (!(error instanceof ApiError)) return "failed"
  if (error.code === "request_open") return "open"
  if (error.code === "already_can_comment") return "can_comment"
  return "failed"
}

export function requestedAtOf(error: unknown): string | undefined {
  return error instanceof ApiError ? error.requestedAt : undefined
}

export function outcomeText(
  outcome: RequestOutcome,
  requestedAt?: string,
  now = Date.now()
): string {
  switch (outcome) {
    case "sent":
      return "Request sent. The people who manage this folder's sharing will see it in their inbox."
    case "open":
      return requestedAt === undefined
        ? "You already asked for comment access here, and the request is still open."
        : `You asked for comment access here ${formatShortRelative(requestedAt, now).toLowerCase()}, and the request is still open.`
    case "can_comment":
      return "You can already comment here. Reload the page to start."
    case "failed":
      return "The request was not sent."
  }
}

export function useRequestCommentAccess(object: AccessObject) {
  return useMutation({
    mutationFn: (note: string) =>
      api
        .POST("/access-requests", {
          body: {
            object,
            ...(note.trim() === "" ? {} : { note: note.trim() }),
          },
        })
        .then(unwrap),
  })
}
