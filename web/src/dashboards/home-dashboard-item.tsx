import { HouseIcon } from "lucide-react"

import { DropdownMenuItem } from "@/components/ui/dropdown-menu"
import {
  useClearHomeDashboard,
  useHomeDashboard,
  useSetHomeDashboard,
} from "@/home/home-dashboard"

export function HomeDashboardItem({ dashboardId }: { dashboardId: string }) {
  const home = useHomeDashboard()
  const set = useSetHomeDashboard()
  const clear = useClearHomeDashboard()
  const isHome =
    home.data?.status === "ok" && home.data.dashboard?.id === dashboardId
  return (
    <DropdownMenuItem
      disabled={home.data === undefined}
      onClick={() => {
        if (isHome) clear.mutate()
        else set.mutate(dashboardId)
      }}
    >
      <HouseIcon aria-hidden="true" />
      {isHome ? "Stop opening on sign-in" : "Open on sign-in"}
    </DropdownMenuItem>
  )
}
