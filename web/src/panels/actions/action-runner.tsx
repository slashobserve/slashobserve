import { useState, type ReactElement } from "react"

import { useActionContext } from "./action-context"
import { ActionDialog, type PendingAction } from "./action-dialog"
import {
  disabledReason,
  type ActionContext,
  type PendingRequest,
  type RunnableAction,
} from "./actions"

export function useActionRunner(): {
  context: ActionContext | null
  reason: string | null
  open: (action: RunnableAction, request: PendingRequest) => void
  dialog: ReactElement | null
} {
  const context = useActionContext()
  const reason = disabledReason(context)
  const [pending, setPending] = useState<PendingAction | null>(null)

  function open(action: RunnableAction, request: PendingRequest) {
    if (reason !== null) return
    setPending({ action, request })
  }

  const dialog =
    pending !== null && context !== null ? (
      <ActionDialog
        dashboardId={context.dashboardId}
        pending={pending}
        onClose={() => setPending(null)}
      />
    ) : null

  return { context, reason, open, dialog }
}
