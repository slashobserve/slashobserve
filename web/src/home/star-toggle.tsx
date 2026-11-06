import { useQuery, useQueryClient } from "@tanstack/react-query"
import { StarIcon } from "lucide-react"

import { latestDashboardQuery } from "@/dashboards/queries"
import { cn } from "@/lib/utils"

import { useStarMutation, useStars } from "./queries"

export function StarToggle({ dashboardId }: { dashboardId: string }) {
  const queryClient = useQueryClient()
  const dashboard = useQuery(latestDashboardQuery(queryClient, dashboardId))
    .data?.dashboard
  const stars = useStars()
  const mutation = useStarMutation()
  const starred = stars.data?.some((star) => star.id === dashboardId) ?? false

  return (
    <button
      type="button"
      aria-pressed={starred}
      disabled={dashboard === undefined || stars.data === undefined}
      onClick={() => {
        if (dashboard === undefined) return
        mutation.mutate({
          target: {
            id: dashboardId,
            title: dashboard.title,
            folder_id: dashboard.folder.id,
            folder_title: dashboard.folder.title,
          },
          starred: !starred,
        })
      }}
      className="flex h-[30px] items-center gap-1.5 rounded-control px-2 text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-pressed:bg-muted"
    >
      <StarIcon
        aria-hidden="true"
        className={cn("size-4", starred && "fill-current")}
      />
      {starred ? "Starred" : "Star"}
    </button>
  )
}
