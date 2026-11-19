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
  const mayEdit = useLatestPermissions(askOn.dashboard_id).edit
  const create = useCreateObserveRequest()
  const navigate = useNavigate()
  const busy = create.isPending || create.isSuccess
  return (
    <div className="flex flex-col gap-2 rounded-card border px-3.5 py-3 text-[13px]">
      <p>
        {mayEdit
          ? `This change is made on the dashboard ${askOn.title}; ask there with the same request.`
          : `This change is made on the dashboard ${askOn.title}, which you cannot edit; someone who can edit it can ask there.`}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {mayEdit ? (
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            aria-busy={busy}
            onClick={() =>
              create.mutate(
                { prompt, dashboardId: askOn.dashboard_id },
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
