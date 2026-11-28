import { Link, useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { sentence } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { useOnDashboard } from "@/dashboards/use-on-dashboard"
import { dashboardLink } from "@/lib/links"
import { useObservation, usePinObservation } from "@/observations/queries"

import { useMakeDashboard, type Finding } from "./queries"

const linkClass =
  "rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"

function toastError(error: Error) {
  toast.error(sentence(error.message))
}

export function PinAction({ finding }: { finding: Finding }) {
  if (finding.observation_id === null) return null
  return (
    <ObservationPin
      observationId={finding.observation_id}
      pinnedBefore={finding.annotation_id !== null}
    />
  )
}

function ObservationPin({
  observationId,
  pinnedBefore,
}: {
  observationId: string
  pinnedBefore: boolean
}) {
  const observation = useObservation(observationId).data?.observation
  if (observation === undefined || observation.dashboard_id === null) {
    return null
  }
  return (
    <PinOnDashboard
      observationId={observationId}
      dashboardId={observation.dashboard_id}
      pinned={observation.pinned || pinnedBefore}
      mayPin={observation.can_pin}
    />
  )
}

function PinOnDashboard({
  observationId,
  dashboardId,
  pinned,
  mayPin,
}: {
  observationId: string
  dashboardId: string
  pinned: boolean
  mayPin: boolean
}) {
  const pin = usePinObservation(observationId)
  const onDashboard = useOnDashboard(dashboardId)
  if (pin.isSuccess || pinned) {
    return (
      <p className="flex flex-wrap items-center gap-x-2 text-[13px]">
        <span>Pinned to the dashboard</span>
        {onDashboard ? null : (
          <Link {...dashboardLink(dashboardId)} className={linkClass}>
            Open dashboard
          </Link>
        )}
      </p>
    )
  }
  if (!mayPin) return null
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={pin.isPending}
      aria-busy={pin.isPending}
      onClick={() => pin.mutate(undefined, { onError: toastError })}
    >
      {pin.isPending ? "Pinning" : "Pin to dashboard"}
    </Button>
  )
}

export function MakeDashboardAction({
  finding,
  chatId,
}: {
  finding: Finding
  chatId: string | undefined
}) {
  const navigate = useNavigate()
  const make = useMakeDashboard(finding.id)
  const busy = make.isPending || make.isSuccess
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={busy}
      aria-busy={busy}
      onClick={() =>
        make.mutate(undefined, {
          onSuccess: (saved) =>
            void navigate({
              ...dashboardLink(saved.dashboard.id, saved.dashboard.title),
              search: chatId === undefined ? {} : { tab: "chat", chat: chatId },
              viewTransition: chatId !== undefined,
            }),
          onError: toastError,
        })
      }
    >
      {busy ? "Making a dashboard" : "Make a dashboard"}
    </Button>
  )
}
