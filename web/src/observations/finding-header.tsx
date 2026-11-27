import type { ReactNode } from "react"

import type { Block, Observation } from "./queries"
import { describeInstant } from "@/lib/time"

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

function hasUnverified(finding: RunFinding): boolean {
  return finding.unverified.length > 0
}

type UnverifiedKind = RunFinding["unverified"][number]["kind"]

function subjectOf(kind: UnverifiedKind, count: number): string {
  switch (kind) {
    case "figure":
      return count === 1 ? "1 figure" : `${count} figures`
    case "component":
      return "the root cause component"
    case "onset":
      return "the onset time"
    case "fact":
      return count === 1 ? "1 cited fact" : `${count} cited facts`
    case "trace_id":
      return count === 1 ? "1 trace id" : `${count} trace ids`
  }
}

export function unverifiedLine(finding: RunFinding): string {
  const counts = new Map<UnverifiedKind, number>()
  for (const item of finding.unverified) {
    counts.set(item.kind, (counts.get(item.kind) ?? 0) + 1)
  }
  const subjects = [...counts].map(([kind, n]) => subjectOf(kind, n))
  const subject =
    subjects.length > 1
      ? `${subjects.slice(0, -1).join(", ")} and ${subjects.at(-1)}`
      : (subjects[0] ?? "")
  const plural =
    subjects.length > 1 ||
    (counts.get("figure") ?? 0) > 1 ||
    (counts.get("fact") ?? 0) > 1 ||
    (counts.get("trace_id") ?? 0) > 1
  const items = finding.unverified.map((item) => item.text).join(", ")
  const sentence = `${subject} ${plural ? "were" : "was"} not found in what the agent read: ${items}`
  return sentence.charAt(0).toUpperCase() + sentence.slice(1)
}

export function headerBlocks(
  blocks: readonly Block[],
  findings: Observation["findings"]
): Map<string, RunFinding> {
  const headers = new Map<string, RunFinding>()
  for (const finding of findings ?? []) {
    if (!hasFindingHeader(finding) && !hasUnverified(finding)) continue
    const answers = blocks
      .filter((b) => b.kind === "text" && b.run_id === finding.run_id)
      .toSorted((a, b) => a.position - b.position)
    const answer = answers.find((b) => b.state === "live") ?? answers[0]
    if (answer !== undefined) headers.set(answer.id, finding)
  }
  return headers
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex gap-2">
      <dt className="w-20 flex-none text-muted-foreground">{label}</dt>
      <dd className="min-w-0 flex-1 break-words">{children}</dd>
    </div>
  )
}

function ItemsRow({
  label,
  items,
}: {
  label: string
  items: readonly string[]
}) {
  return (
    <Row label={label}>
      <ul className="list-disc pl-4">
        {items.map((item, index) => (
          <li key={index}>{item}</li>
        ))}
      </ul>
    </Row>
  )
}

function UnverifiedLine({ finding }: { finding: RunFinding }) {
  const line = unverifiedLine(finding)
  return <p className="text-[12px] text-muted-foreground">{line}</p>
}

export function FindingHeader({ finding }: { finding: RunFinding }) {
  return (
    <div className="mb-3 flex flex-col gap-1">
      {hasFindingHeader(finding) ? <FindingSummary finding={finding} /> : null}
      {hasUnverified(finding) ? <UnverifiedLine finding={finding} /> : null}
    </div>
  )
}

function FindingSummary({ finding }: { finding: RunFinding }) {
  const cause = finding.root_cause
  return (
    <section
      aria-label="Finding summary"
      className="flex flex-col gap-1 rounded-card border border-border px-3 py-2 text-[13px]"
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
          <Row label="Started">{describeInstant(finding.onset)}</Row>
        )}
        {finding.impact === null ? null : (
          <Row label="Impact">{finding.impact}</Row>
        )}
        {finding.confidence === null ? null : (
          <Row label="Confidence">{finding.confidence}</Row>
        )}
        {finding.ruled_out.length === 0 ? null : (
          <ItemsRow label="Ruled out" items={finding.ruled_out} />
        )}
        {finding.next_steps.length === 0 ? null : (
          <ItemsRow label="Next steps" items={finding.next_steps} />
        )}
      </dl>
    </section>
  )
}
