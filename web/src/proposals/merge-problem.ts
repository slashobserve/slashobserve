import { ApiError, type ProblemTexts } from "@/api/errors"

const staleTexts: Record<string, string> = {
  proposal_changed:
    "This proposal has a newer revision. Review it and merge again.",
  version_conflict:
    "The dashboard changed while merging. Review the conflicts and merge again.",
  invalid_transition: "This proposal is already closed.",
}

export const mergeProblems: ProblemTexts = { codes: staleTexts }

export function mergeLeftProposalStale(error: unknown): boolean {
  return error instanceof ApiError && Object.hasOwn(staleTexts, error.code)
}
