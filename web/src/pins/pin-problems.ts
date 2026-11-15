import {
  dashboardProblems,
  problemCountText,
  type HealthItem,
} from "@/health/health-labels"

export function pinProblemText(item: HealthItem | undefined): string | null {
  if (item === undefined) return null
  const count = dashboardProblems(item).length
  return count === 0 ? null : problemCountText(count)
}
