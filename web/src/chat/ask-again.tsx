import { RotateCcwIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  describeSubmitError,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"

import { planSend, type Opens } from "./send-plan"

export function AskAgainButton({
  prompt,
  dashboardId,
  followsId,
  disabled,
  onSent,
}: {
  prompt: string
  dashboardId?: string
  followsId?: string
  disabled: boolean
  onSent: (request: ObserveRequest, opens: Opens) => void | Promise<void>
}) {
  const create = useCreateObserveRequest()

  async function askAgain() {
    const plan = planSend({
      command: null,
      text: prompt,
      dashboardId,
      followsId,
    })
    if ("error" in plan) return
    try {
      const request = await create.mutateAsync(plan.input)
      await onSent(request, plan.opens)
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    }
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={disabled || create.isPending}
      aria-busy={create.isPending}
      onClick={() => void askAgain()}
    >
      <RotateCcwIcon aria-hidden="true" />
      Ask again
    </Button>
  )
}
