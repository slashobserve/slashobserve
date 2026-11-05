type AgentModel = { model: string | null; effort: string | null }

export function agentLabel(agent: AgentModel | undefined): string | null {
  if (agent === undefined || agent.model === null) return null
  return agent.effort === null
    ? agent.model
    : `${agent.model}, ${agent.effort} effort`
}
