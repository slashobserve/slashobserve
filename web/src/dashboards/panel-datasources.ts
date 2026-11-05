import {
  allPanels,
  isDataPanel,
  isPanelQuery,
  resolveDatasourceRef,
  type Dashboard,
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
      isPanelQuery(query) ? query.datasource : undefined
    ),
  ]) {
    if (ref) uids.add(resolveDatasourceRef(ref, selections) ?? ref)
  }
  return [...uids]
}

export function mixesDatasources(
  spec: Dashboard,
  selections: Record<string, string[]>
): boolean {
  const uids = new Set(
    allPanels(spec).flatMap(({ panel }) =>
      panelDatasourceUids(panel, selections)
    )
  )
  return uids.size > 1
}
