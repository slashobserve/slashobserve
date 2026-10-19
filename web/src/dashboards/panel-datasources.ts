import { isDataPanel, type Panel } from "@/spec"

export function panelDatasourceUids(panel: Panel): string[] {
  if (!isDataPanel(panel)) return []
  const uids = new Set<string>()
  for (const uid of [
    panel.datasource,
    ...panel.queries.map((query) => query.datasource),
  ]) {
    if (uid) uids.add(uid)
  }
  return [...uids]
}
