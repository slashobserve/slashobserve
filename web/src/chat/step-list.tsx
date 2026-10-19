import type { StatusTone } from "@/components/status-label"
import { groupSteps, type Step } from "@/observe/request-model"

export const phaseLabels: Record<Step["phase"], string> = {
  discover: "Discovering",
  lint: "Checking queries",
  query: "Running queries",
  save: "Saving",
  repair: "Repairing",
}

const iconProps = {
  width: 13,
  height: 13,
  viewBox: "0 0 24 24",
  fill: "none",
  strokeLinecap: "round",
  strokeLinejoin: "round",
  "aria-hidden": true,
} as const

export function ToneIcon({ tone }: { tone: StatusTone }) {
  switch (tone) {
    case "ok":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-status-ok"
          strokeWidth={2.4}
        >
          <path d="m5 12 5 5 9-10" />
        </svg>
      )
    case "warning":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-status-warning"
          strokeWidth={2.2}
        >
          <path d="M12 3 2 20h20L12 3z" />
          <path d="M12 10v4M12 17h.01" />
        </svg>
      )
    case "critical":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-status-critical"
          strokeWidth={2.2}
        >
          <circle cx={12} cy={12} r={9} />
          <path d="M12 8v5M12 16h.01" />
        </svg>
      )
    case "running":
      return (
        <svg
          {...iconProps}
          className="flex-none stroke-brand motion-safe:animate-spin"
          strokeWidth={2.4}
        >
          <path d="M21 12a9 9 0 1 1-6.2-8.6" />
        </svg>
      )
    case "none":
      return null
  }
}

type Props = { steps: Step[]; working: boolean; reconnecting: boolean }

export function StepList({ steps, working, reconnecting }: Props) {
  return (
    <ol
      aria-label="Progress"
      aria-live="polite"
      aria-relevant="additions"
      className="flex flex-col gap-1.5 empty:hidden"
    >
      {groupSteps(steps).map(({ step, count }, index) => {
        const failed = step.status === "error"
        return (
          <li key={index} className="flex items-start gap-2 text-[13px]">
            <span className="flex h-[1.45em] items-center">
              <ToneIcon tone={failed ? "warning" : "ok"} />
            </span>
            <span className="flex min-w-0 flex-wrap gap-x-2">
              <span>
                {phaseLabels[step.phase]}
                {failed ? <span className="sr-only"> needed a fix</span> : null}
              </span>
              <span className="min-w-0 wrap-anywhere text-muted-foreground">
                {count > 1 ? `${count} times, latest: ` : ""}
                {step.message}
              </span>
            </span>
          </li>
        )
      })}
      {working ? (
        <li className="flex items-center gap-2 text-[13px]">
          <ToneIcon tone="running" />
          <span>Working</span>
        </li>
      ) : null}
      {working && reconnecting ? (
        <li className="pl-[21px] text-[13px] text-muted-foreground">
          Reconnecting...
        </li>
      ) : null}
    </ol>
  )
}
