import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import type { Opens } from "@/conversation/send-plan"
import type { SentResult } from "@/conversation/use-composer"
import {
  describeSubmitError,
  useCreateObserveRequest,
  useCreatePending,
  type ObserveRequest,
} from "@/observe/queries"

export function useOpenSent(append?: (request: ObserveRequest) => void) {
  const navigate = useNavigate()
  return async (request: ObserveRequest, opens: Opens) => {
    if (opens === "same" && append !== undefined) {
      append(request)
      return
    }
    await navigate({
      to: "/c/$requestId",
      params: { requestId: request.id },
    })
  }
}

export function useSendExample() {
  const create = useCreateObserveRequest()
  const openSent = useOpenSent()

  async function send(prompt: string, dashboardId?: string) {
    try {
      const request = await create.mutateAsync({
        prompt,
        kind: "build",
        ...(dashboardId === undefined ? {} : { dashboardId }),
      })
      await openSent(request, "chat")
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    }
  }

  return { send, pending: useCreatePending() }
}

export function useOpenSentTurn(append?: (request: ObserveRequest) => void) {
  const open = useOpenSent(append)
  return (sent: SentResult) =>
    sent.via === "chat" ? open(sent.request, sent.opens) : undefined
}
