import { lazy, Suspense, useMemo } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { RelativeTime } from "@/components/relative-time"
import { Button, buttonVariants } from "@/components/ui/button"
import { dashboardLink, findingLink } from "@/lib/links"
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
  const pin = usePinFinding(finding)
  if (finding.dashboard_id === null) return null
  if (pin.isSuccess || finding.annotation_id !== null) {
    return (
      <p className="flex flex-wrap items-center gap-x-2 text-[13px]">
        <span>Pinned to the dashboard</span>
        <Link {...dashboardLink(finding.dashboard_id)} className={linkClass}>
          Open dashboard
        </Link>
      </p>
    )
  }
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

function MakeDashboardAction({ finding }: { finding: Finding }) {
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
            void navigate(
              dashboardLink(saved.dashboard.id, saved.dashboard.title)
            ),
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
    text: "text-sm leading-[1.55]",
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
  variant = "card",
}: {
  finding: Finding
  showOpenLink: boolean
  variant?: FindingVariant
}) {
  const panels = useMemo(() => evidencePanels(finding), [finding])
  const layout = variants[variant]
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
        <PinAction finding={finding} />
        {panels.length > 0 ? <MakeDashboardAction finding={finding} /> : null}
        {showOpenLink ? (
          <Link
            {...findingLink(finding.id)}
            className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
          >
            Open finding
          </Link>
        ) : null}
      </div>
    </div>
  )
}
