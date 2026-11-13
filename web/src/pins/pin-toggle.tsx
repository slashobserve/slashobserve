import { useQuery, useQueryClient } from "@tanstack/react-query"
import { PinIcon } from "lucide-react"

import { latestDashboardQuery } from "@/dashboards/queries"
import { cn } from "@/lib/utils"

import { usePinMutation, usePins } from "./queries"

export function PinToggle({ dashboardId }: { dashboardId: string }) {
  const queryClient = useQueryClient()
  const dashboard = useQuery(latestDashboardQuery(queryClient, dashboardId))
    .data?.dashboard
  const pins = usePins()
  const mutation = usePinMutation()
  const pinned = pins.data?.some((pin) => pin.id === dashboardId) ?? false
  const label = pinned ? "Unpin from sidebar" : "Pin to sidebar"

  return (
    <button
      type="button"
      aria-pressed={pinned}
      aria-label={label}
      title={label}
      disabled={dashboard === undefined || pins.data === undefined}
      onClick={() => {
        if (dashboard === undefined) return
        mutation.mutate({
          target: {
            id: dashboardId,
            title: dashboard.title,
            folder_id: dashboard.folder.id,
            folder_title: dashboard.folder.title,
          },
          pinned: !pinned,
        })
      }}
      className="flex h-[30px] items-center gap-1.5 rounded-control px-2 text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-pressed:bg-muted"
    >
      <PinIcon
        aria-hidden="true"
        className={cn("size-4", pinned && "fill-current")}
      />
    </button>
  )
}
