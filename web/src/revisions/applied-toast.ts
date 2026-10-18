const applied = new Set<string>()

export function versionToastId(dashboardId: string, version: number): string {
  return `version-${dashboardId}-${version}`
}

export function markVersionApplied(id: string) {
  applied.add(id)
}

export function isVersionApplied(id: string): boolean {
  return applied.has(id)
}
