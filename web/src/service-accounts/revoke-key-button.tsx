import { useEffect, useRef, useState, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

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

import {
  problemText,
  useRevokeKey,
  type ServiceAccount,
  type ServiceAccountKey,
} from "./queries"

type Props = {
  account: ServiceAccount
  serviceKey: ServiceAccountKey
  finalFocus: RefObject<HTMLElement | null>
}

export function RevokeKeyButton({ account, serviceKey, finalFocus }: Props) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const revoked = useRef(false)
  const revoke = useRevokeKey(account.id)

  useEffect(
    () => () => {
      const active = document.activeElement
      if (revoked.current && (active === null || active === document.body)) {
        finalFocus.current?.focus()
      }
    },
    [finalFocus]
  )

  function changeOpen(next: boolean) {
    if (!next && revoke.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      revoke.reset()
    }
  }

  function confirm() {
    setProblem(null)
    revoke.mutate(serviceKey.id, {
      onSuccess: () => {
        revoked.current = true
        toast.success(`Revoked key ${serviceKey.name}`)
        setOpen(false)
      },
      onError: (error) => setProblem(problemText(error, { what: "revoke" })),
    })
  }

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="px-3 font-normal"
        aria-label={`Revoke key ${serviceKey.name} for ${account.name}`}
        onClick={() => {
          revoked.current = false
          setOpen(true)
        }}
      >
        Revoke
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent
          finalFocus={() => (revoked.current ? finalFocus.current : true)}
        >
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Revoke key {serviceKey.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Anything using this key (it starts with{" "}
              <span className="font-mono text-xs">
                {serviceKey.display_prefix}
              </span>
              ) stops working immediately. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={revoke.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={revoke.isPending}
              focusableWhenDisabled
              aria-busy={revoke.isPending}
              onClick={confirm}
            >
              {revoke.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Revoke key
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
