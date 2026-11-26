import { Link, useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  mayEditBesidesContent,
  useLatestPermissions,
  useLatestProvisioned,
} from "@/dashboards/permissions"
import { useOnDashboard } from "@/dashboards/use-on-dashboard"
import { dashboardLink } from "@/lib/links"
import { sentence } from "@/observe/queries"

import { useMakeDashboard, usePinFinding, type Finding } from "./queries"

const linkClass =
  "rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"

function toastError(error: Error) {
  toast.error(sentence(error.message))
}

export function PinAction({ finding }: { finding: Finding }) {
  if (finding.dashboard_id === null) return null
  return <PinOnDashboard finding={finding} dashboardId={finding.dashboard_id} />
}

function PinOnDashboard({
  finding,
  dashboardId,
}: {
  finding: Finding
  dashboardId: string
}) {
  const pin = usePinFinding(finding)
  const onDashboard = useOnDashboard(dashboardId)
  const mayPin = mayEditBesidesContent(
    useLatestPermissions(dashboardId),
    useLatestProvisioned(dashboardId)
  )
  if (pin.isSuccess || finding.annotation_id !== null) {
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
