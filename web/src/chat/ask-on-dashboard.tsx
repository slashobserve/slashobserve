import { Link, useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { Button, buttonVariants } from "@/components/ui/button"
import { useLatestPermissions } from "@/dashboards/permissions"
import type { Finding } from "@/findings/queries"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { describeSubmitError, useCreateObserveRequest } from "@/observe/queries"

type AskOn = NonNullable<Finding["ask_on"]>

export function AskOnDashboard({
  askOn,
  prompt,
}: {
  askOn: AskOn
  prompt: string
}) {
  const mayAsk = useLatestPermissions(askOn.dashboard_id).comment
  const create = useCreateObserveRequest()
  const navigate = useNavigate()
  const busy = create.isPending || create.isSuccess
  return (
    <div className="flex flex-col gap-2 rounded-card border px-3.5 py-3 text-[13px]">
      <p>
        {mayAsk
          ? `This request is about the dashboard ${askOn.title}. Ask there and the agent answers about it; a change it suggests is made only when you choose Revise the dashboard.`
          : `This request is about the dashboard ${askOn.title}, where you cannot ask the agent.`}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {mayAsk ? (
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            aria-busy={busy}
            onClick={() =>
              create.mutate(
                { prompt, dashboardId: askOn.dashboard_id, kind: "ask" },
                {
                  onSuccess: (request) =>
                    void navigate({
                      to: "/c/$requestId",
                      params: { requestId: request.id },
                    }),
                  onError: (cause) =>
                    toast.error(describeSubmitError(cause).message),
                }
              )
            }
          >
            <span className="max-w-64 truncate">
              {busy ? "Asking" : `Ask this on ${askOn.title}`}
            </span>
          </Button>
        ) : null}
        <Link
          {...dashboardLink(askOn.dashboard_id, askOn.title)}
          className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
        >
          Open dashboard
        </Link>
      </div>
    </div>
  )
}
