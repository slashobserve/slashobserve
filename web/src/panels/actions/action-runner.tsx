import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactElement,
} from "react"

import type { LinkScope } from "../field-config"
import { useActionContext } from "./action-context"
import { ActionDialog, type PendingAction } from "./action-dialog"
import {
  disabledReason,
  type ActionContext,
  type PendingRequest,
  type RunnableAction,
} from "./actions"
import { fieldActionRequest, type FieldAction } from "./field-actions"

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

export type StableActionRunner = {
  context: ActionContext | null
  reason: string | null
  open: (action: RunnableAction, request: PendingRequest) => void
  panelId: string
  fromMs: number
  toMs: number
}

export function useStableActionRunner(
  panelId: string,
  { from, to }: { from: string; to: string }
): { runner: StableActionRunner; dialog: ReactElement | null } {
  const { context, reason, open, dialog } = useActionRunner()
  const latestOpen = useRef(open)
  useEffect(() => {
    latestOpen.current = open
  })
  const stableOpen = useCallback<StableActionRunner["open"]>(
    (action, request) => latestOpen.current(action, request),
    []
  )
  const runner = useMemo(
    () => ({
      context,
      reason,
      open: stableOpen,
      panelId,
      fromMs: Date.parse(from),
      toMs: Date.parse(to),
    }),
    [context, reason, stableOpen, panelId, from, to]
  )
  return { runner, dialog }
}

export function runFieldAction(
  runner: StableActionRunner,
  action: FieldAction,
  scope: LinkScope
) {
  if (runner.context === null) return
  runner.open(
    action,
    fieldActionRequest({
      context: runner.context,
      panelId: runner.panelId,
      action,
      scope,
      fromMs: runner.fromMs,
      toMs: runner.toMs,
    })
  )
}
