import { trimSpace } from "../brief/gofmt"
import { figure, onsetText, short } from "./render"
import type { Candidate, Lead } from "./types"

export const maxLeads = 5

const maxLeadRows = 2
const maxLeadRunes = 160

function clipRunes(s: string, n: number): string {
  const r = [...s]
  if (r.length <= n) {
    return s
  }
  return trimSpace(r.slice(0, n - 1).join("")) + "…"
}

export function leads(candidates: readonly Candidate[]): Lead[] {
  return candidates.slice(0, maxLeads).map((c) => {
    const moved: string[] = []
    const signals: string[] = []
    for (const row of c.rows) {
      if (moved.length < maxLeadRows) {
        moved.push(
          `${row.signal} ${figure(row.window, row.unit)} from ${figure(row.baseline, row.unit)} (${short(row.rung)})`
        )
      }
      if (!signals.includes(row.signal)) {
        signals.push(row.signal)
      }
    }
    const text = `${c.service}: ${moved.join("; ")}; onset ${onsetText(c.onset)}`
    const next = `Read ${c.service} ${signals.join(" and ")} before and after the onset, then close it as cause, knock_on, background, transient or unrelated`
    return {
      id: c.id,
      service: c.service,
      text: clipRunes(text, maxLeadRunes),
      next: clipRunes(next, maxLeadRunes),
    }
  })
}
