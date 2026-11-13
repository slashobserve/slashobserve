import { useCallback, useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"

import { useAgentInfo } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import {
  panelProblems,
  repairOutcomeText,
  type HealthItem,
  type HealthProblem,
} from "./health-labels"
import { dashboardsHealthQuery, useRequestHealthRepair } from "./queries"

export function useDashboardHealth(dashboardId: string) {
  const health = useQuery(dashboardsHealthQuery([dashboardId]))
  const agent = useAgentInfo()
  const { mutate, isPending } = useRequestHealthRepair()
  const repair = useCallback(
    (onDone?: () => void) =>
      mutate(dashboardId, {
        onSuccess: (outcome) => {
          toast.success(repairOutcomeText(outcome))
          onDone?.()
        },
        onError: (error) => toast.error(loadProblemText(error)),
      }),
    [mutate, dashboardId]
  )
  return {
    item: health.data?.get(dashboardId) ?? null,
    agentAvailable: agent.data?.available ?? false,
    repair,
    pending: isPending,
  }
}

export type PanelHealth = {
  problems: readonly HealthProblem[]
  item: HealthItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
  blockedReason: string | null
}

const noPanelHealth: ReadonlyMap<string, PanelHealth> = new Map()

export function usePanelHealth(
  dashboardId: string,
  drawnVersion: number,
  blockedReason: string | null
): ReadonlyMap<string, PanelHealth> {
  const { item, agentAvailable, repair, pending } =
    useDashboardHealth(dashboardId)
  const onRepair = useCallback(() => repair(), [repair])
  return useMemo(() => {
    if (item === null) return noPanelHealth
    const byPanel = panelProblems(item, drawnVersion)
    if (byPanel.size === 0) return noPanelHealth
    return new Map(
      [...byPanel].map(([panelId, problems]) => [
        panelId,
        { problems, item, agentAvailable, pending, onRepair, blockedReason },
      ])
    )
  }, [item, drawnVersion, agentAvailable, pending, onRepair, blockedReason])
}
