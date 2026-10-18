import { useState } from "react"
import { useNavigate, useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError, NetworkError } from "@/api/errors"
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
import { invalidateSession } from "@/lib/auth"
import { sentence } from "@/observe/queries"

import { useSignOutEverywhere, type User } from "./queries"

function problemText(error: unknown): string {
  if (error instanceof NetworkError) {
    return "Could not reach the server. Check your connection and try again."
  }
  if (error instanceof ApiError) {
    if (error.status === 404) return "This person no longer exists."
    return sentence(error.message)
  }
  return "Something went wrong. Try again."
}

export function SignOutEverywhereButton({ user }: { user: User }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const signOut = useSignOutEverywhere()
  const isMe = principal.id === user.id

  function changeOpen(next: boolean) {
    if (!next && signOut.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      signOut.reset()
    }
  }

  function leave() {
    invalidateSession()
    void navigate({ to: "/login" })
  }

  function confirm() {
    setProblem(null)
    signOut.mutate(user.id, {
      onSuccess: () => {
        if (isMe) {
          leave()
          return
        }
        toast.success(`Signed out ${user.name} everywhere`)
        setOpen(false)
      },
      onError: (error) => {
        if (isMe && error instanceof ApiError && error.status === 401) {
          leave()
          return
        }
        setProblem(problemText(error))
      },
    })
  }

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="font-normal"
        aria-label={`Sign out ${user.name} everywhere`}
        onClick={() => setOpen(true)}
      >
        Sign out everywhere
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Sign out {user.name} everywhere?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This ends every session of{" "}
              <span className="wrap-anywhere">{user.email}</span> on this
              server. They can sign in again.
              {isMe ? " This includes this browser." : null}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={signOut.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={signOut.isPending}
              focusableWhenDisabled
              aria-busy={signOut.isPending}
              onClick={confirm}
            >
              {signOut.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Sign out everywhere
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
