import { ApiError, problemText } from "@/api/errors"

import { reviseProblems, type RevisionAsked } from "./answer-card"
import type { Place } from "./delivery"
import { planRevise, type Subject } from "./send-plan"
import { useDeliver, type SentResult } from "./use-composer"

export function useReviseAnswer({
  subject,
  place,
  revised,
  findingId,
  onSent,
}: {
  subject: Subject | null
  place: Place
  revised: Subject | null
  findingId: string
  onSent?: (sent: SentResult) => Promise<void>
}) {
  const { deliver, pending } = useDeliver(subject)

  async function revise(): Promise<RevisionAsked> {
    const plan = planRevise(place, revised, findingId, crypto.randomUUID())
    if ("error" in plan) return plan.error || problemText(null, reviseProblems)
    let sent: SentResult
    try {
      sent = await deliver(plan)
    } catch (error) {
      if (error instanceof ApiError && error.code === "already_applied") {
        return true
      }
      return problemText(error, reviseProblems)
    }
    await onSent?.(sent)
    return true
  }

  return { revise, pending }
}
