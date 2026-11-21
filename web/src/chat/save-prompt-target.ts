import { requestOrigin, type OriginFields } from "@/observe/request-origin"

export function canSaveAsPrompt(request: OriginFields): boolean {
  return requestOrigin(request) === "observe"
}

export function savePromptButtonId(requestId: string): string {
  return `history-save-prompt-${requestId}`
}
