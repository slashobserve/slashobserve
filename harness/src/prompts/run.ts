import facts from "../../../internal/agent/prompts/facts.md" with { type: "text" }
import investigate from "../../../internal/agent/prompts/investigate.md" with { type: "text" }
import ledger from "../../../internal/agent/prompts/ledger.md" with { type: "text" }
import explain from "../../../internal/agent/prompts/playbook/explain.md" with { type: "text" }
import fullPlaybook from "../../../internal/agent/prompts/playbook/full.md" with { type: "text" }
import incident from "../../../internal/agent/prompts/playbook/incident.md" with { type: "text" }
import inventory from "../../../internal/agent/prompts/playbook/inventory.md" with { type: "text" }
import logs from "../../../internal/agent/prompts/playbook/logs.md" with { type: "text" }
import measure from "../../../internal/agent/prompts/playbook/measure.md" with { type: "text" }
import traces from "../../../internal/agent/prompts/playbook/traces.md" with { type: "text" }
import scopeText from "../../../internal/agent/prompts/scope.md" with { type: "text" }
import systemText from "../../../internal/agent/prompts/system.md" with { type: "text" }
import exampleModule from "../../../spec/examples/checkout-service-health.json" with { type: "text" }
import type { Task } from "../route/route"

// The text attribute makes Bun import a string; TypeScript still types the parsed JSON.
const example = exampleModule as unknown as string

const playbooks: Record<Exclude<Task, "">, string> = {
  measure,
  logs,
  traces,
  incident,
  inventory,
  explain,
}

const fullOrder = [
  "incident",
  "measure",
  "logs",
  "traces",
  "inventory",
  "explain",
] as const

const enforcedRules = [
  "Give one value per quantity, copied at the precision the tool returned",
  "When two reads give different figures for it, make one more read that settles it",
]

export type Variant = {
  task: Task
  facts: boolean
  ledger: boolean
}

export function system(): string {
  return systemText
}

export function scope(): string {
  return scopeText
}

function trimmed(s: string): string {
  return s.replace(/\n+$/, "")
}

function playbookFor(task: Task): string {
  if (task !== "") {
    return trimmed(playbooks[task])
  }
  return [
    trimmed(fullPlaybook),
    ...fullOrder.map((t) => trimmed(playbooks[t])),
  ].join("\n\n")
}

function withoutEnforcedRules(body: string): string {
  return body
    .split(/(?<=\n)/)
    .filter((line) => !enforcedRules.some((rule) => line.startsWith(rule)))
    .join("")
}

export function investigateBody(v: Variant): string {
  const core = v.facts ? withoutEnforcedRules(investigate) : investigate
  const parts = [trimmed(core), playbookFor(v.task)]
  if (v.facts) {
    parts.push(trimmed(facts))
  }
  if (v.ledger) {
    parts.push(trimmed(ledger))
  }
  return parts.join("\n\n") + "\n"
}

export function investigateFor(v: Variant, guide: string): string {
  return (
    investigateBody(v) +
    "\n" +
    trimmed(scopeText) +
    "\n\n# Authoring guide\n" +
    trimmed(guide) +
    "\n"
  )
}

export function buildSystem(guide: string): string {
  return (
    trimmed(systemText) +
    "\n\n" +
    trimmed(scopeText) +
    "\n\n# Authoring guide\n" +
    trimmed(guide) +
    "\n\n# Example Spec (format only)\n```json\n" +
    trimmed(compactJSON(example)) +
    "\n```\n"
  )
}

export function compactJSON(body: string): string {
  let out = ""
  let inString = false
  let escaped = false
  for (const c of body) {
    if (inString) {
      out += c
      if (escaped) {
        escaped = false
      } else if (c === "\\") {
        escaped = true
      } else if (c === '"') {
        inString = false
      }
    } else if (c === '"') {
      inString = true
      out += c
    } else if (c !== " " && c !== "\t" && c !== "\n" && c !== "\r") {
      out += c
    }
  }
  return out
}
