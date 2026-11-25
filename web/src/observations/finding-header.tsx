import type { Block, Observation } from "./queries"
import { formatDateTime } from "@/lib/time"

export type RunFinding = NonNullable<Observation["findings"]>[number]

const kindLabels: Record<
  NonNullable<RunFinding["root_cause"]>["kind"],
  string
> = {
  deploy: "Deploy",
  config_or_flag: "Config or flag",
  dependency: "Dependency",
  resource: "Resource",
  data: "Data",
  traffic: "Traffic",
  infrastructure: "Infrastructure",
  unknown: "Unknown",
}

export function hasFindingHeader(finding: RunFinding): boolean {
  return (
    finding.root_cause !== null ||
    finding.onset !== null ||
    finding.impact !== null ||
    finding.confidence !== null ||
    finding.ruled_out.length > 0 ||
    finding.next_steps.length > 0 ||
    finding.no_incident
  )
}

export function headerBlocks(
  blocks: readonly Block[],
  findings: Observation["findings"]
): Map<string, RunFinding> {
  const headers = new Map<string, RunFinding>()
  for (const finding of findings ?? []) {
    if (!hasFindingHeader(finding)) continue
    const answers = blocks
      .filter((b) => b.kind === "text" && b.run_id === finding.run_id)
      .toSorted((a, b) => a.position - b.position)
    const answer = answers.find((b) => b.state === "live") ?? answers[0]
    if (answer !== undefined) headers.set(answer.id, finding)
  }
  return headers
}

function Row({ label, children }: { label: string; children: string }) {
  return (
    <div className="flex gap-2">
      <dt className="w-20 flex-none text-muted-foreground">{label}</dt>
      <dd className="min-w-0 flex-1 break-words">{children}</dd>
    </div>
  )
}

export function FindingHeader({ finding }: { finding: RunFinding }) {
  const cause = finding.root_cause
  return (
    <section
      aria-label="Finding summary"
      className="mb-3 flex flex-col gap-1 rounded-card border border-border px-3 py-2 text-[13px]"
    >
      {finding.no_incident ? (
        <p className="font-medium">No incident found</p>
      ) : null}
      {cause === null ? null : (
        <p>
          <span className="font-medium">{cause.component}</span>
          <span className="text-muted-foreground">
            {" "}
            · {kindLabels[cause.kind]}
          </span>
          {" - "}
          {cause.summary}
        </p>
      )}
      <dl className="flex flex-col gap-0.5">
        {finding.onset === null ? null : (
          <Row label="Started">{formatDateTime(finding.onset)}</Row>
        )}
        {finding.impact === null ? null : (
          <Row label="Impact">{finding.impact}</Row>
        )}
        {finding.confidence === null ? null : (
          <Row label="Confidence">{finding.confidence}</Row>
        )}
        {finding.ruled_out.length === 0 ? null : (
          <Row label="Ruled out">{finding.ruled_out.join("; ")}</Row>
        )}
        {finding.next_steps.length === 0 ? null : (
          <Row label="Next steps">{finding.next_steps.join("; ")}</Row>
        )}
      </dl>
    </section>
  )
}
