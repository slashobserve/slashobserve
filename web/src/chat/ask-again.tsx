import { LayoutDashboardIcon, RotateCcwIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  describeSubmitError,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"

import { askOnLabel, type AskOn } from "./ask-on"
import { planSend, type Opens } from "./send-plan"

type SendProps = {
  prompt: string
  dashboardId?: string
  followsId?: string
  disabled: boolean
  onSent: (request: ObserveRequest, opens: Opens) => void | Promise<void>
  afterSent?: () => void
}

function useAsk(
  { prompt, dashboardId, followsId, onSent, afterSent }: SendProps,
  opensOverride?: Opens
) {
  const create = useCreateObserveRequest()

  async function ask() {
    const plan = planSend({
      command: null,
      text: prompt,
      dashboardId,
      followsId,
    })
    if ("error" in plan) return
    try {
      const request = await create.mutateAsync(plan.input)
      await onSent(request, opensOverride ?? plan.opens)
      afterSent?.()
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    }
  }

  return { ask, pending: create.isPending }
}

export function AskAgainButton(props: SendProps) {
  const { ask, pending } = useAsk(props)
  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={props.disabled || pending}
      focusableWhenDisabled
      aria-busy={pending}
      onClick={() => void ask()}
    >
      <RotateCcwIcon aria-hidden="true" />
      Ask again
    </Button>
  )
}

export function AskOnButton({
  askOn,
  ...props
}: Omit<SendProps, "dashboardId" | "followsId"> & { askOn: AskOn }) {
  const { ask, pending } = useAsk(
    { ...props, dashboardId: askOn.dashboardId },
    "chat"
  )
  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={props.disabled || pending}
      focusableWhenDisabled
      aria-busy={pending}
      onClick={() => void ask()}
    >
      <LayoutDashboardIcon aria-hidden="true" />
      {askOnLabel(askOn.title)}
    </Button>
  )
}
