import { lazy, Suspense, useMemo, type ReactNode } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { CopyButton } from "@/components/copy-button"
import { RelativeTime } from "@/components/relative-time"
import { Button, buttonVariants } from "@/components/ui/button"
import { useLatestPermissions } from "@/dashboards/permissions"
import { dashboardLink, observationLink } from "@/lib/links"
import { describeRange } from "@/lib/time"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import { EvidencePanel } from "./evidence-panel"
import {
  evidencePanels,
  useMakeDashboard,
  usePinFinding,
  type Finding,
} from "./queries"

const FindingMarkdown = lazy(() =>
  import("./finding-markdown").then((module) => ({
    default: module.FindingMarkdown,
  }))
)

const evidenceHeight = 220

const linkClass =
  "rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"

function toastError(error: Error) {
  toast.error(sentence(error.message))
}

function PinAction({ finding }: { finding: Finding }) {
  if (finding.dashboard_id === null) return null
  return <PinOnDashboard finding={finding} dashboardId={finding.dashboard_id} />
}

function PinOnDashboard({
  finding,
  dashboardId,
}: {
  finding: Finding
  dashboardId: string
}) {
  const pin = usePinFinding(finding)
  const mayPin = useLatestPermissions(dashboardId).edit
  if (pin.isSuccess || finding.annotation_id !== null) {
    return (
      <p className="flex flex-wrap items-center gap-x-2 text-[13px]">
        <span>Pinned to the dashboard</span>
        <Link {...dashboardLink(dashboardId)} className={linkClass}>
          Open dashboard
        </Link>
      </p>
    )
  }
  if (!mayPin) return null
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={pin.isPending}
      aria-busy={pin.isPending}
      onClick={() => pin.mutate(undefined, { onError: toastError })}
    >
      {pin.isPending ? "Pinning" : "Pin to dashboard"}
    </Button>
  )
}

function MakeDashboardAction({
  finding,
  chatId,
}: {
  finding: Finding
  chatId: string | undefined
}) {
  const navigate = useNavigate()
  const make = useMakeDashboard(finding.id)
  const busy = make.isPending || make.isSuccess
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={busy}
      aria-busy={busy}
      onClick={() =>
        make.mutate(undefined, {
          onSuccess: (saved) =>
            void navigate({
              ...dashboardLink(saved.dashboard.id, saved.dashboard.title),
              search: chatId === undefined ? {} : { tab: "chat", chat: chatId },
            }),
          onError: toastError,
        })
      }
    >
      {busy ? "Making a dashboard" : "Make a dashboard"}
    </Button>
  )
}

const variants = {
  card: {
    frame: "gap-3 rounded-card border px-3.5 py-3",
    text: "text-sm leading-normal",
    grid: "@[640px]:grid-cols-2",
  },
  page: {
    frame: "gap-4",
    text: "max-w-[680px] text-[15px] leading-relaxed",
    grid: "@[760px]:grid-cols-2",
  },
}

type FindingVariant = keyof typeof variants

export function FindingCard({
  finding,
  showOpenLink,
  chatId,
  actions,
  variant = "card",
}: {
  finding: Finding
  showOpenLink: boolean
  chatId?: string
  actions?: ReactNode
  variant?: FindingVariant
}) {
  const panels = useMemo(() => evidencePanels(finding), [finding])
  const layout = variants[variant]
  const observationId = finding.observation_id
  return (
    <div className={cn("flex flex-col", layout.frame)}>
      <div className={layout.text}>
        <Suspense
          fallback={<p className="whitespace-pre-wrap">{finding.markdown}</p>}
        >
          <FindingMarkdown markdown={finding.markdown} />
        </Suspense>
      </div>
      {panels.length > 0 ? (
        <div className="@container">
          <div className={cn("grid grid-cols-1 gap-2.5", layout.grid)}>
            {panels.map((panel) => (
              <EvidencePanel
                key={panel.id}
                findingId={finding.id}
                panel={panel}
                from={finding.from}
                to={finding.to}
                height={evidenceHeight}
              />
            ))}
          </div>
        </div>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {variant === "page" ? (
          `Evidence from ${describeRange(finding.from, finding.to)}.`
        ) : (
          <>
            Found <RelativeTime iso={finding.created_at} />,{" "}
            {describeRange(finding.from, finding.to)}
          </>
        )}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {observationId !== null ? <PinAction finding={finding} /> : null}
        {panels.length > 0 && observationId !== null ? (
          <MakeDashboardAction finding={finding} chatId={chatId} />
        ) : null}
        <CopyButton
          text={finding.markdown}
          label="Copy answer"
          variant="ghost"
          size="sm"
        >
          Copy answer
        </CopyButton>
        {actions}
        {showOpenLink && observationId !== null ? (
          <Link
            {...observationLink(observationId)}
            className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
          >
            Open observation
          </Link>
        ) : null}
      </div>
    </div>
  )
}
