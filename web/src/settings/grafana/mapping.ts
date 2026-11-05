import type { components } from "@/api/schema.gen"

export type GrafanaDatasource = components["schemas"]["GrafanaDatasource"]
export type Picks = Record<string, string>

export function supported(datasources: GrafanaDatasource[]) {
  return datasources.filter((datasource) => datasource.kind !== null)
}

export function unsupported(datasources: GrafanaDatasource[]) {
  return datasources.filter((datasource) => datasource.kind === null)
}

export function prefilledMapping(datasources: GrafanaDatasource[]): Picks {
  const picks: Picks = {}
  for (const datasource of supported(datasources)) {
    if (datasource.matched_uid !== null) {
      picks[datasource.uid] = datasource.matched_uid
    }
  }
  return picks
}

export function mappingToSend(
  datasources: GrafanaDatasource[],
  picks: Picks
): Picks {
  const mapping: Picks = {}
  for (const datasource of supported(datasources)) {
    const pick = picks[datasource.uid]
    if (
      pick !== undefined &&
      pick !== datasource.matched_uid &&
      datasource.candidates.includes(pick)
    ) {
      mapping[datasource.uid] = pick
    }
  }
  return mapping
}

export function openMatches(
  datasources: GrafanaDatasource[],
  picks: Picks
): number {
  return supported(datasources).filter((datasource) => {
    const pick = picks[datasource.uid]
    return (
      datasource.matched_uid === null &&
      (pick === undefined || !datasource.candidates.includes(pick))
    )
  }).length
}
