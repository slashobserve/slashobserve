import {
  isDataPanel,
  isExpressionQuery,
  resolveDatasourceRef,
  type Panel,
} from "@/spec"

export function panelDatasourceUids(
  panel: Panel,
  selections: Record<string, string[]>
): string[] {
  if (!isDataPanel(panel)) return []
  const uids = new Set<string>()
  for (const ref of [
    panel.datasource,
    ...panel.queries.map((query) =>
      isExpressionQuery(query) ? undefined : query.datasource
    ),
  ]) {
    if (ref) uids.add(resolveDatasourceRef(ref, selections) ?? ref)
  }
  return [...uids]
}
