import { useId } from "react"
import { Link } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { Badge } from "@/components/ui/badge"
import { markOf, type Actor } from "@/lib/author"

import { agentLink } from "./agent-ref"
import type { AgentsOverview } from "./queries"
import { scopeLine, statCells } from "./stats"

const observeActor: Pick<
  Actor,
  "kind" | "id" | "name" | "origin" | "client" | "run_id"
> = {
  kind: "user",
  id: "observe",
  name: "Observe Agent",
  origin: null,
  client: null,
  run_id: "observe",
}

export const observeAgentMark = markOf(observeActor)

function modelLine(agent: AgentsOverview["observe_agent"]): string {
  return agent.model === null
    ? "No model configured"
    : [agent.provider, agent.model].filter(Boolean).join(" ")
}

export function ObserveAgentCard({
  agent,
}: {
  agent: AgentsOverview["observe_agent"]
}) {
  const headingId = useId()
  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col gap-4 rounded-card border p-4"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <AuthorMark mark={observeAgentMark} />
        <h2 id={headingId} className="text-sm font-semibold">
          <Link
            {...agentLink({ kind: "observe" })}
            className="rounded-sm no-underline outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            Observe Agent
          </Link>
        </h2>
        <Badge variant={agent.enabled ? "default" : "outline"}>
          {agent.enabled ? "On" : "Off"}
        </Badge>
        <span className="text-[13px] text-muted-foreground">
          {modelLine(agent)}
        </span>
      </div>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-3 @min-[560px]:grid-cols-4">
        {statCells(agent.last_7_days).map((cell) => (
          <div key={cell.label} className="flex flex-col gap-0.5">
            <dt className="text-xs text-muted-foreground">{cell.label}</dt>
            <dd className="text-lg font-semibold">{cell.value}</dd>
          </div>
        ))}
      </dl>
      <p className="text-xs text-muted-foreground">{scopeLine(agent.scope)}</p>
    </section>
  )
}
