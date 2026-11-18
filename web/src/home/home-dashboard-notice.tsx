import { Button } from "@/components/ui/button"

import { useClearHomeDashboard, useHomeDashboard } from "./home-dashboard"

const reasons = {
  gone: "The dashboard you chose to open on sign-in was deleted, so Home opens instead.",
  unreadable:
    "You can no longer view the dashboard you chose to open on sign-in, so Home opens instead.",
} as const

export function HomeDashboardNotice() {
  const home = useHomeDashboard().data
  const clear = useClearHomeDashboard()
  if (home?.status !== "gone" && home?.status !== "unreadable") return null
  return (
    <div
      role="status"
      className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 text-[13px] text-muted-foreground"
    >
      <p className="min-w-0 flex-1">{reasons[home.status]}</p>
      <Button
        variant="outline"
        size="sm"
        disabled={clear.isPending}
        onClick={() => clear.mutate()}
      >
        Clear it
      </Button>
    </div>
  )
}
