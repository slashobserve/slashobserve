import { lazy, Suspense, useMemo, type ReactNode } from "react"
import { Link } from "@tanstack/react-router"

import { CopyButton } from "@/components/copy-button"
import { RelativeTime } from "@/components/relative-time"
import { buttonVariants } from "@/components/ui/button"
import { observationLink } from "@/lib/links"
import { describeRange } from "@/lib/time"
import { cn } from "@/lib/utils"

import { EvidencePanel } from "./evidence-panel"
import { MakeDashboardAction, PinAction } from "./finding-actions"
import { evidencePanels, type Finding } from "./queries"

const FindingMarkdown = lazy(() =>
  import("./finding-markdown").then((module) => ({
    default: module.FindingMarkdown,
  }))
)

const evidenceHeight = 220

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
  chatId?: string | undefined
  actions?: ReactNode
  variant?: FindingVariant | undefined
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
