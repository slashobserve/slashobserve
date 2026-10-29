import type { RequestSummary } from "@/observe/queries"

export function canSaveAsPrompt(
  request: Pick<RequestSummary, "source">
): boolean {
  return request.source === "observe"
}

export function savePromptButtonId(requestId: string): string {
  return `history-save-prompt-${requestId}`
}
