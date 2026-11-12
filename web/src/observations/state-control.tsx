import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { toast } from "sonner"

import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { blockWriteProblem } from "./errors"
import { ResolveDialog, stateActionId } from "./resolve-dialog"
import { useReopenObservation, type Observation } from "./queries"

type Props = {
  observation: Observation
  folder: { id: string; title: string }
}

export function StateControl({ observation, folder }: Props) {
  const [resolving, setResolving] = useState(false)
  const reopen = useReopenObservation(observation.id)
  const open = observation.state === "open"
  const canResolve = observation.allowed_actions.includes("resolve")
  const canReopen = observation.allowed_actions.includes("reopen")

  function reopenIt() {
    if (reopen.isPending) return
    reopen.mutate(undefined, {
      onError: (error) => toast.error(blockWriteProblem(error).text),
    })
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[13px]">
        <span className="inline-flex items-center gap-1.5 font-medium">
          <span
            aria-hidden="true"
            className={cn(
              "size-[7px] rounded-full",
              open ? "bg-status-ok" : "bg-muted-foreground"
            )}
          />
          {open ? "Open" : "Resolved"}
        </span>
        <span className="text-muted-foreground">
          Started by {observation.created_by.label},{" "}
          <RelativeTime iso={observation.created_at} />
        </span>
        <span className="text-muted-foreground">
          In{" "}
          <Link
            to="/f/$folderId"
            params={{ folderId: folder.id }}
            className="rounded-control text-foreground underline-offset-2 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            {folder.title}
          </Link>
        </span>
        {canResolve ? (
          <Button
            id={stateActionId}
            variant="outline"
            size="sm"
            onClick={() => setResolving(true)}
          >
            Resolve
          </Button>
        ) : null}
        {canReopen ? (
          <Button
            id={stateActionId}
            variant="outline"
            size="sm"
            aria-disabled={reopen.isPending || undefined}
            aria-busy={reopen.isPending || undefined}
            onClick={reopenIt}
          >
            Reopen
          </Button>
        ) : null}
      </div>
      {!open && observation.summary !== null ? (
        <div className="flex flex-col gap-0.5">
          <p className="text-xs text-muted-foreground">Summary</p>
          <p className="text-sm">{observation.summary}</p>
        </div>
      ) : null}
      <ResolveDialog
        observation={observation}
        open={resolving}
        onOpenChange={setResolving}
      />
    </div>
  )
}
