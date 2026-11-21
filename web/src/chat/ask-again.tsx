import { RotateCcwIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  describeSubmitError,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"

import { planSend, type Opens } from "@/conversation/send-plan"

export function AskAgainButton({
  prompt,
  dashboardId,
  chatId,
  disabled,
  onSent,
  afterSent,
}: {
  prompt: string
  dashboardId?: string
  chatId?: string
  disabled: boolean
  onSent: (request: ObserveRequest, opens: Opens) => void | Promise<void>
  afterSent?: () => void
}) {
  const create = useCreateObserveRequest()

  async function askAgain() {
    const plan = planSend(
      { kind: "chat", chatId: chatId ?? null },
      dashboardId === undefined ? null : { kind: "dashboard", id: dashboardId },
      { to: "agent", visibleTo: "me" },
      { body: prompt, clientId: crypto.randomUUID(), revises: true }
    )
    if ("error" in plan || plan.via !== "chat.turn") return
    try {
      const request = await create.mutateAsync(plan.input)
      await onSent(request, plan.opens)
      afterSent?.()
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    }
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={disabled || create.isPending}
      focusableWhenDisabled
      aria-busy={create.isPending}
      onClick={() => void askAgain()}
    >
      <RotateCcwIcon aria-hidden="true" />
      Ask again
    </Button>
  )
}
