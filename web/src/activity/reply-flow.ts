export async function runReply(input: {
  sendToAgent: boolean
  postReply: () => Promise<boolean>
  sendRequest: () => Promise<void>
  isOpen: () => boolean
  onClose: () => void
  onRequestError: (failure: unknown) => void
}): Promise<"finished" | "retry"> {
  if (!(await input.postReply())) return "retry"
  if (input.sendToAgent) {
    try {
      await input.sendRequest()
    } catch (failure) {
      input.onRequestError(failure)
      return "retry"
    }
  }
  if (input.isOpen()) input.onClose()
  return "finished"
}

export function closeUnlessBusy(busy: boolean, onClose: () => void): boolean {
  if (busy) return false
  onClose()
  return true
}

export function dismissLabel(replySaved: boolean): "Cancel" | "Done" {
  return replySaved ? "Done" : "Cancel"
}
