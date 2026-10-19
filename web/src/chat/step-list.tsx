import { StatusIcon } from "@/components/status-label"
import { groupSteps, type Step } from "@/observe/request-model"

export const phaseLabels: Record<Step["phase"], string> = {
  discover: "Discovering",
  lint: "Checking queries",
  query: "Running queries",
  save: "Saving",
  repair: "Repairing",
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
              <StatusIcon tone={failed ? "warning" : "ok"} />
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
          <StatusIcon tone="running" />
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
