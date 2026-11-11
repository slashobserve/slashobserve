import { useRef, useState } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { loadProblemText } from "@/service-accounts/queries"

import { useDisconnect } from "./queries"
import {
  agentName,
  disconnectLabel,
  disconnectTitle,
  type ConnectedAgent,
  type GrantScope,
} from "./rows"

function isGone(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

export function DisconnectButton({
  grant,
  scope,
}: {
  grant: ConnectedAgent
  scope: GrantScope
}) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const disconnect = useDisconnect(scope)

  function changeOpen(next: boolean) {
    if (!next && disconnect.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      disconnect.reset()
    }
  }

  function confirm() {
    setProblem(null)
    disconnect.mutate(grant.id, {
      onSuccess: () => {
        toast.success(`Disconnected ${agentName(grant.client.name)}`)
        setOpen(false)
      },
      onError: (error) => {
        if (isGone(error)) {
          setOpen(false)
          return
        }
        setProblem(loadProblemText(error))
      },
    })
  }

  return (
    <>
      <Button
        ref={trigger}
        variant="outline"
        size="sm"
        className="font-normal"
        aria-label={disconnectLabel(grant, scope)}
        onClick={() => setOpen(true)}
      >
        Disconnect
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent finalFocus={trigger}>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              {disconnectTitle(grant.client.name)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              It stops working at once. Connect it again from the agent to use
              it.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={disconnect.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={disconnect.isPending}
              focusableWhenDisabled
              aria-busy={disconnect.isPending}
              onClick={confirm}
            >
              {disconnect.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Disconnect
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
