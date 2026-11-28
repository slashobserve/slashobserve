import { RotateCcwIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  describeSubmitError,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"

import { planSend, type Opens, type Subject } from "@/conversation/send-plan"

type AskInChat = {
  subject?: Subject | null | undefined
  chatId?: string | undefined
  onSent: (request: ObserveRequest, opens: Opens) => void | Promise<void>
  afterSent?: (() => void) | undefined
}

export function useAskInChat({
  subject = null,
  chatId,
  onSent,
  afterSent,
}: AskInChat) {
  const create = useCreateObserveRequest()

  async function ask(prompt: string) {
    const plan = planSend(
      { kind: "chat", chatId: chatId ?? null },
      subject,
      { to: "agent", visibleTo: "me" },
      { body: prompt, clientId: crypto.randomUUID() }
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

  return { ask, pending: create.isPending }
}

export function AskAgainButton({
  prompt,
  subject,
  chatId,
  disabled,
  onSent,
  afterSent,
}: {
  prompt: string
  subject?: Subject | null
  chatId?: string | undefined
  disabled: boolean
  onSent: AskInChat["onSent"]
  afterSent?: () => void
}) {
  const { ask, pending } = useAskInChat({ subject, chatId, onSent, afterSent })

  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={disabled || pending}
      focusableWhenDisabled
      aria-busy={pending}
      onClick={() => void ask(prompt)}
    >
      <RotateCcwIcon aria-hidden="true" />
      Ask again
    </Button>
  )
}
