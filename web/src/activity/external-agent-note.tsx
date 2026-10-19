import { CopyButton } from "@/components/copy-button"

import { claudeCodePrompt, commentsListCommand } from "./copy-prompt"

type Props = {
  dashboardId: string
  title: string
  clientName: string
}

export function ExternalAgentNote({ dashboardId, title, clientName }: Props) {
  const command = commentsListCommand(dashboardId)
  const prompt = claudeCodePrompt({
    dashboardId,
    title,
    url: `${window.location.origin}/dashboards/${dashboardId}`,
  })
  return (
    <section
      aria-label={`Waiting for ${clientName}`}
      className="flex flex-col gap-2 rounded-[8px] border px-[11px] py-[9px] text-xs text-muted-foreground"
    >
      <div className="flex items-start gap-2">
        <p className="flex-1">
          No model is connected to this server. Open comments wait for{" "}
          {clientName}, which reads them over MCP. Copy a prompt for it:
        </p>
        <CopyButton
          text={prompt}
          label="Copy prompt"
          variant="outline"
          icon={false}
          className="h-[26px] flex-none px-[9px] text-xs"
        >
          Copy prompt
        </CopyButton>
      </div>
      <div className="flex max-w-full min-w-0 items-center gap-1 self-start rounded-control border px-2 py-1">
        <code className="truncate font-mono text-xs text-foreground">
          {command}
        </code>
        <CopyButton
          text={command}
          label="Copy command"
          variant="ghost"
          size="icon-xs"
        />
      </div>
    </section>
  )
}
