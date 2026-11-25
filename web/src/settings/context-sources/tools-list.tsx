import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsProblem } from "@/settings/settings-card"

import {
  contextProblemText,
  hasReadOnlyToApprove,
  toolStatus,
} from "./problems"
import {
  contextToolsQuery,
  useApproveContextTool,
  useApproveReadOnlyTools,
  useRevokeContextTool,
  type ContextTool,
} from "./queries"

function ChangedDefinition({ tool }: { tool: ContextTool }) {
  if (tool.seen === null) return null
  return (
    <dl className="mt-1.5 grid gap-x-3 gap-y-1 text-xs sm:grid-cols-[auto_1fr]">
      <dt className="text-muted-foreground">Approved</dt>
      <dd className="wrap-anywhere">{tool.description || "No description"}</dd>
      <dt className="text-muted-foreground">Now</dt>
      <dd className="wrap-anywhere">
        {tool.seen.description || "No description"}
      </dd>
    </dl>
  )
}

function ToolRow({ uid, tool }: { uid: string; tool: ContextTool }) {
  const approve = useApproveContextTool()
  const revoke = useRevokeContextTool()
  const status = toolStatus(tool)
  const busy = approve.isPending || revoke.isPending
  const canApprove = tool.allowed_actions.includes("approve")
  const canRevoke = tool.allowed_actions.includes("revoke")

  return (
    <li className="flex flex-wrap items-start gap-x-4 gap-y-2 border-t px-4 py-2.5 first:border-t-0">
      <div className="min-w-[200px] flex-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="font-mono text-[13px] wrap-anywhere">
            {tool.name}
          </span>
          <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
        </div>
        <p className="mt-0.5 text-xs wrap-anywhere text-muted-foreground">
          {status.detail}
        </p>
        {tool.state === "changed" ? (
          <ChangedDefinition tool={tool} />
        ) : tool.description === "" ? null : (
          <p className="mt-1 line-clamp-2 text-xs wrap-anywhere">
            {tool.description}
          </p>
        )}
      </div>
      <div className="flex flex-none gap-2">
        {canApprove ? (
          <Button
            size="sm"
            variant={tool.state === "changed" ? "default" : "outline"}
            className="narrow:h-11"
            disabled={busy}
            aria-label={`${tool.state === "changed" ? "Approve the new" : "Approve"} ${tool.name}`}
            onClick={() =>
              approve.mutate(
                { uid, tool },
                {
                  onError: (error) => toast.error(contextProblemText(error)),
                }
              )
            }
          >
            {tool.state === "changed" ? "Approve change" : "Approve"}
          </Button>
        ) : null}
        {canRevoke ? (
          <Button
            size="sm"
            variant="outline"
            className="narrow:h-11"
            disabled={busy}
            aria-label={`Revoke ${tool.name}`}
            onClick={() =>
              revoke.mutate(
                { uid, name: tool.name },
                {
                  onError: (error) => toast.error(contextProblemText(error)),
                }
              )
            }
          >
            Revoke
          </Button>
        ) : null}
      </div>
    </li>
  )
}

export function ToolsList({ uid }: { uid: string }) {
  const query = useQuery(contextToolsQuery(uid))
  const approveAll = useApproveReadOnlyTools()

  if (query.isError && query.data === undefined) {
    return (
      <SettingsProblem
        title="Could not load the tools"
        detail={contextProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  if (query.data === undefined) {
    return (
      <div className="p-4">
        <Skeleton className="h-12 w-full" />
      </div>
    )
  }
  if (query.data.length === 0) {
    return (
      <p className="px-4 py-3 text-[13px] text-muted-foreground">
        No tools yet. Discover the server's tools first.
      </p>
    )
  }
  return (
    <div>
      {hasReadOnlyToApprove(query.data) ? (
        <div className="flex flex-wrap items-center gap-3 px-4 py-2.5">
          <Button
            size="sm"
            className="narrow:h-11"
            disabled={approveAll.isPending}
            aria-busy={approveAll.isPending}
            onClick={() =>
              approveAll.mutate(uid, {
                onSuccess: ({ approved }) =>
                  toast.success(
                    `Approved ${approved.length} ${approved.length === 1 ? "tool" : "tools"}`
                  ),
                onError: (error) => toast.error(contextProblemText(error)),
              })
            }
          >
            {approveAll.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Approve all read-only
          </Button>
          <span className="text-xs text-muted-foreground">
            Approves every tool to review that the server marks read-only.
          </span>
        </div>
      ) : null}
      <ul className="border-t">
        {query.data.map((tool) => (
          <ToolRow key={tool.name} uid={uid} tool={tool} />
        ))}
      </ul>
    </div>
  )
}
