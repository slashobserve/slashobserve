import { useId } from "react"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { versionChangesSearch } from "@/history/search"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"

import { repairAction, type HealthItem } from "./health-labels"

type RepairControlProps = {
  item: HealthItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
  blockedReason?: string | null
  showOpen?: boolean
  label?: string
}

export function RepairControl({
  item,
  agentAvailable,
  pending,
  onRepair,
  blockedReason = null,
  showOpen = false,
  label = "Repair",
}: RepairControlProps) {
  const action = repairAction(item, agentAvailable)
  switch (action.kind) {
    case "queued":
      return <StatusLabel tone="running">Repair queued</StatusLabel>
    case "running":
      return <StatusLabel tone="running">Repairing</StatusLabel>
    case "repaired":
      return (
        <span className="inline-flex flex-wrap items-center justify-end gap-x-2 gap-y-1">
          <StatusLabel tone="ok">Repaired</StatusLabel>
          <Link
            to="/d/$dashboardId/versions"
            params={{ dashboardId: item.id }}
            search={versionChangesSearch(action.version)}
            className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
          >
            See changes
          </Link>
        </span>
      )
    case "failed":
      return (
        <span className="flex flex-col items-end gap-1">
          <StatusLabel tone="critical">Repair failed</StatusLabel>
          {action.error === null ? null : (
            <span className="max-w-64 text-left text-xs wrap-anywhere whitespace-normal text-muted-foreground">
              {action.error}
            </span>
          )}
          {action.canRetry ? (
            <RepairButton
              disabledReason={action.disabledReason ?? blockedReason}
              pending={pending}
              onRepair={onRepair}
              label={label}
            />
          ) : null}
        </span>
      )
    case "repair":
      return (
        <RepairButton
          disabledReason={action.disabledReason ?? blockedReason}
          pending={pending}
          onRepair={onRepair}
          label={label}
        />
      )
    case "open":
      return showOpen ? (
        <Link
          {...dashboardLink(item.id, item.title)}
          className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
        >
          Open
        </Link>
      ) : null
  }
}

function RepairButton({
  disabledReason,
  pending,
  onRepair,
  label,
}: {
  disabledReason: string | null
  pending: boolean
  onRepair: () => void
  label: string
}) {
  const reasonId = useId()
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={disabledReason !== null || pending}
        focusableWhenDisabled
        aria-busy={pending}
        aria-describedby={disabledReason === null ? undefined : reasonId}
        title={disabledReason ?? undefined}
        onClick={onRepair}
      >
        {pending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : null}
        {label}
      </Button>
      {disabledReason === null ? null : (
        <span id={reasonId} className="sr-only">
          {disabledReason}
        </span>
      )}
    </>
  )
}
