import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"

import { useDatasources } from "@/datasources/queries"
import { roleAtLeast } from "@/lib/roles"
import { useAgentInfo } from "@/observe/queries"
import { dashboardTitlesQuery } from "@/shell/palette-queries"

import { examplePrompts, noDashboards } from "./examples"

export function useExamples() {
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const dashboards = useQuery(dashboardTitlesQuery())
  const { principal } = useRouteContext({ from: "/_authed" })

  const examples = examplePrompts({
    types: (datasources.data ?? []).filter((d) => d.healthy).map((d) => d.type),
    dashboards: dashboards.data ?? noDashboards,
    canBuild: roleAtLeast(principal.role, "editor"),
    personId: principal.id,
  })
  return {
    examples,
    agentReady: !agent.isPending && agent.data?.available !== false,
    agentOff: agent.data?.available === false,
  }
}
