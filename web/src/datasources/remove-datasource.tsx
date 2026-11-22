import { useRef, useState } from "react"
import { Link } from "@tanstack/react-router"
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
import { dashboardLink } from "@/lib/links"

import { DatasourceError, useDeleteDatasource } from "./admin-queries"
import { saveProblemText, usersSummary } from "./problems"
import type { DatasourceStatus } from "./queries"

export function RemoveDatasourceButton({
  datasource,
}: {
  datasource: DatasourceStatus
}) {
  const [open, setOpen] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  const remove = useDeleteDatasource()
  const inUse =
    remove.error instanceof DatasourceError &&
    remove.error.code === "datasource_in_use"
      ? remove.error
      : null
  const problem =
    remove.isError && inUse === null ? saveProblemText(remove.error) : null

  function changeOpen(next: boolean) {
    if (!next && remove.isPending) return
    setOpen(next)
    if (!next) remove.reset()
  }

  function confirm() {
    remove.mutateAsync(datasource.uid).then(
      () => {
        toast.success(`Removed ${datasource.name}`)
        setOpen(false)
      },
      (error) => {
        if (error instanceof DatasourceError && error.status === 404) {
          setOpen(false)
        }
      }
    )
  }

  return (
    <>
      <Button
        ref={trigger}
        variant="outline"
        size="xs"
        aria-label={`Remove ${datasource.uid}`}
        onClick={() => setOpen(true)}
      >
        Remove
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent finalFocus={trigger}>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Remove {datasource.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Queries stop using it at once. Add it again to restore it; its
              credentials are deleted and cannot be recovered.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {inUse === null ? null : (
            <Alert variant="destructive">
              <AlertDescription>
                <p>
                  It cannot be removed while these use it. Change them to
                  another datasource first.
                </p>
                <ul className="list-disc pl-4">
                  {usersSummary(inUse.usedBy, inUse.hidden).map((line) => (
                    <li key={line.key} className="wrap-anywhere">
                      {line.dashboard === undefined ? (
                        line.text
                      ) : (
                        <Link
                          {...dashboardLink(
                            line.dashboard.id,
                            line.dashboard.title
                          )}
                          className="underline underline-offset-2"
                        >
                          {line.text}
                        </Link>
                      )}
                    </li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}
          {problem === null ? null : (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              {inUse === null ? "Cancel" : "Close"}
            </AlertDialogCancel>
            {inUse === null ? (
              <AlertDialogAction
                variant="destructive"
                disabled={remove.isPending}
                focusableWhenDisabled
                aria-busy={remove.isPending}
                onClick={confirm}
              >
                {remove.isPending ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="motion-safe:animate-spin"
                  />
                ) : null}
                Remove
              </AlertDialogAction>
            ) : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
