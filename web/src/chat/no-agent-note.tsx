export function NoAgentNote() {
  return (
    <div className="w-full rounded-card border bg-background px-4 py-3.5 text-[13px]">
      <p className="font-medium">No model is connected to this server.</p>
      <p className="text-muted-foreground">
        Set <code className="font-mono text-xs">SLASHOBSERVE_LLM_PROVIDER</code>{" "}
        and restart the server to build dashboards from a chat.
      </p>
    </div>
  )
}
