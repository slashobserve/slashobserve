export const saveTools: ReadonlySet<string> = new Set([
  "create_dashboard",
  "update_dashboard",
  "update_panels",
  "import_grafana_dashboard",
  "save_finding",
])

export function isSave(tool: string): boolean {
  return saveTools.has(tool)
}
