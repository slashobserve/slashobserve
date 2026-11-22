import { useRef, useState } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"
import { EllipsisIcon, LoaderCircleIcon } from "lucide-react"
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import {
  actionsFor,
  lifecycleProblemText,
  removalText,
  usePersonAction,
  type PersonAction,
} from "./person-lifecycle"
import type { User } from "./queries"

const labels: Record<PersonAction, string> = {
  deactivate: "Deactivate",
  reactivate: "Reactivate",
  remove: "Remove",
}

const done: Record<PersonAction, string> = {
  deactivate: "Deactivated",
  reactivate: "Reactivated",
  remove: "Removed",
}

function destructive(action: PersonAction): boolean {
  return action !== "reactivate"
}

function keysNote(count: number): string {
  const keys = count === 1 ? "1 API key" : `${count} API keys`
  return `They created ${keys}, which keep working because a key acts as its service account. Revoke them on the API keys page if they should stop.`
}

function Description({ user, action }: { user: User; action: PersonAction }) {
  if (action === "remove") {
    return (
      <AlertDialogDescription>
        This deletes <span className="wrap-anywhere">{user.email}</span> for
        good.{" "}
        {user.personal_items === null ? null : removalText(user.personal_items)}{" "}
        Their saved prompts, stars, inbox and folder grants go too. Dashboards,
        observations and comments they made in shared folders stay under their
        name, and API keys they created keep working. If they sign in again they
        join as a new person.
      </AlertDialogDescription>
    )
  }
  if (action === "reactivate") {
    return (
      <AlertDialogDescription>
        <span className="wrap-anywhere">{user.email}</span> can sign in again
        with the role the sign-in rules give them. Agents they had connected
        stay disconnected until they connect them again.
      </AlertDialogDescription>
    )
  }
  return (
    <AlertDialogDescription>
      This ends every session of{" "}
      <span className="wrap-anywhere">{user.email}</span>, disconnects every
      agent they connected and stops them from signing in until an Admin
      reactivates them. Their dashboards, observations and comments stay where
      they are.
      {user.key_count > 0 ? (
        <>
          {" "}
          {keysNote(user.key_count)}{" "}
          <Link to="/keys" className="underline underline-offset-2">
            Open API keys
          </Link>
        </>
      ) : null}
    </AlertDialogDescription>
  )
}

export function PersonMenu({ user }: { user: User }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const trigger = useRef<HTMLButtonElement>(null)
  const [confirming, setConfirming] = useState<PersonAction | null>(null)
  const change = usePersonAction()
  const actions = actionsFor(user, principal.id)

  if (actions.length === 0) return null

  function changeOpen(next: boolean) {
    if (!next && change.isPending) return
    if (!next) {
      setConfirming(null)
      change.reset()
    }
  }

  function confirm() {
    if (confirming === null) return
    const action = confirming
    change.mutateAsync({ id: user.id, action }).then(
      () => {
        toast.success(`${done[action]} ${user.name}`)
        setConfirming(null)
      },
      () => {}
    )
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              ref={trigger}
              variant="outline"
              size="icon-sm"
              className="narrow:size-11"
              aria-label={`More for ${user.name}`}
            />
          }
        >
          <EllipsisIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {actions.map((action) => (
            <DropdownMenuItem
              key={action}
              variant={destructive(action) ? "destructive" : "default"}
              onClick={() => setConfirming(action)}
            >
              {labels[action]}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <AlertDialog open={confirming !== null} onOpenChange={changeOpen}>
        <AlertDialogContent finalFocus={trigger}>
          {confirming !== null ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle className="wrap-anywhere">
                  {labels[confirming]} {user.name}?
                </AlertDialogTitle>
                <Description user={user} action={confirming} />
              </AlertDialogHeader>
              {change.isError ? (
                <Alert variant="destructive">
                  <AlertDescription>
                    {lifecycleProblemText(change.error)}
                  </AlertDescription>
                </Alert>
              ) : null}
              <AlertDialogFooter>
                <AlertDialogCancel aria-disabled={change.isPending}>
                  Cancel
                </AlertDialogCancel>
                <AlertDialogAction
                  variant={destructive(confirming) ? "destructive" : "default"}
                  disabled={change.isPending}
                  focusableWhenDisabled
                  aria-busy={change.isPending}
                  onClick={confirm}
                >
                  {change.isPending ? (
                    <LoaderCircleIcon
                      aria-hidden="true"
                      className="motion-safe:animate-spin"
                    />
                  ) : null}
                  {labels[confirming]}
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          ) : null}
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
