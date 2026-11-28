export type Hypothesis = {
  id: string
  text: string
  status: string
  closing?: string
  knock_on_of?: string
  evidence?: string[]
  next?: string
}

const hypothesesOpen =
  "<hypotheses>\nYour ledger so far; send the whole ledger with note_hypotheses when it changes, and do not repeat a read that ruled one out.\n"
const hypothesesClosed = "</hypotheses>"

function statusOf(h: Hypothesis): string {
  if (h.closing === "knock_on") {
    return h.status + ", knock_on of " + (h.knock_on_of ?? "")
  }
  if (h.closing) {
    return h.status + ", " + h.closing
  }
  return h.status
}

export function hypothesesText(ledger: readonly Hypothesis[]): string {
  if (ledger.length === 0) {
    return ""
  }
  let out = hypothesesOpen
  for (const h of ledger) {
    out += "- " + h.id + " [" + statusOf(h) + "] " + h.text + "\n"
    for (const line of h.evidence ?? []) {
      out += "  evidence: " + line + "\n"
    }
    if (h.next) {
      out += "  next: " + h.next + "\n"
    }
  }
  return out + hypothesesClosed
}
