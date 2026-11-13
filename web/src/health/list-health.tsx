import { createContext, useContext, useMemo, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"

import { useAgentInfo } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { repairOutcomeText, type HealthItem } from "./health-labels"
import { dashboardsHealthQuery, useRequestHealthRepair } from "./queries"

type ListHealth = {
  items: ReadonlyMap<string, HealthItem> | undefined
  pending: boolean
  failed: boolean
  retrying: boolean
  retry: () => void
  agentAvailable: boolean
  pendingId: string | null
  requestRepair: (dashboardId: string) => void
}

const nothingKnown: ListHealth = {
  items: undefined,
  pending: false,
  failed: false,
  retrying: false,
  retry: () => {},
  agentAvailable: false,
  pendingId: null,
  requestRepair: () => {},
}

const ListHealthContext = createContext<ListHealth>(nothingKnown)

export function ListHealthProvider({
  ids,
  children,
}: {
  ids: readonly string[]
  children: ReactNode
}) {
  const health = useQuery(dashboardsHealthQuery(ids))
  const agent = useAgentInfo()
  const repair = useRequestHealthRepair()
  const { mutate } = repair
  const { refetch } = health
  const value = useMemo<ListHealth>(
    () => ({
      items: health.data,
      pending: health.isPending && ids.length > 0,
      failed: health.isError,
      retrying: health.isFetching,
      retry: () => void refetch(),
      agentAvailable: agent.data?.available ?? false,
      pendingId: repair.isPending ? repair.variables : null,
      requestRepair: (dashboardId) =>
        mutate(dashboardId, {
          onSuccess: (outcome) => toast.success(repairOutcomeText(outcome)),
          onError: (error) => toast.error(loadProblemText(error)),
        }),
    }),
    [
      health.data,
      health.isPending,
      health.isError,
      health.isFetching,
      refetch,
      ids.length,
      agent.data?.available,
      repair.isPending,
      repair.variables,
      mutate,
    ]
  )
  return <ListHealthContext value={value}>{children}</ListHealthContext>
}

export function useListHealth(id: string): {
  item: HealthItem | null
  pending: boolean
} {
  const { items, pending } = useContext(ListHealthContext)
  return { item: items?.get(id) ?? null, pending }
}

export function useListHealthRepair() {
  const { agentAvailable, pendingId, requestRepair } =
    useContext(ListHealthContext)
  return { agentAvailable, pendingId, requestRepair }
}

export function useListHealthLoad() {
  const { failed, retrying, retry } = useContext(ListHealthContext)
  return { failed, retrying, retry }
}
