export const maxSummaryLength = 500

export function summaryProblem(text: string): string | null {
  const trimmed = text.trim()
  if (trimmed === "") return "Write a summary"
  if (/[\r\n]/.test(trimmed)) return "Keep it to one line"
  if ([...trimmed].length > maxSummaryLength) {
    return "Keep it under 500 characters"
  }
  return null
}
