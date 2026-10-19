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
      className="mx-4 mt-3 rounded-[8px] border px-3 py-2.5 text-[13px]"
    >
      <p>
        No model is connected to this server. Open comments wait for{" "}
        {clientName}, which reads them over MCP. Copy a prompt for it:
      </p>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <CopyButton
          text={prompt}
          label="Copy prompt"
          variant="outline"
          className="h-[26px] text-xs"
        >
          Copy prompt
        </CopyButton>
        <div className="flex min-w-0 items-center gap-1 rounded-control border px-2 py-1">
          <code className="truncate font-mono text-xs">{command}</code>
          <CopyButton
            text={command}
            label="Copy command"
            variant="ghost"
            size="icon-xs"
          />
        </div>
      </div>
    </section>
  )
}
