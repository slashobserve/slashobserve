import {
  dashboardKeys,
  observeKeys,
  type PanelDataParams,
  type VariableValuesParams,
} from "@/api/keys"
import { stableJson } from "@/lib/json"

export type PanelSource =
  | { kind: "version"; dashboardId: string; version: number }
  | { kind: "build"; requestId: string; saves: number }

export function panelFingerprint(panel: unknown): string {
  return stableJson(panel)
}

export function sourceScope(source: PanelSource): string {
  return source.kind === "version"
    ? `${source.dashboardId}:${source.version}`
    : `build:${source.requestId}`
}

export function loadRangeScope(
  source: PanelSource,
  range: { from?: string; to?: string }
): string {
  return source.kind === "version"
    ? sourceScope(source)
    : `${sourceScope(source)}:${range.from ?? ""}:${range.to ?? ""}`
}

export function stateScope(source: PanelSource): string {
  return source.kind === "version"
    ? source.dashboardId
    : `build:${source.requestId}`
}

export function panelDataKey(
  source: PanelSource,
  panelId: string,
  fingerprint: string,
  params: PanelDataParams
) {
  return source.kind === "version"
    ? dashboardKeys.panelData(
        source.dashboardId,
        source.version,
        panelId,
        params
      )
    : observeKeys.buildPanelData(source.requestId, panelId, fingerprint, params)
}

export function variableValuesKey(
  source: PanelSource,
  name: string,
  fingerprint: string,
  params: VariableValuesParams
) {
  return source.kind === "version"
    ? dashboardKeys.variableValues(
        source.dashboardId,
        source.version,
        name,
        params
      )
    : observeKeys.buildVariableValues(
        source.requestId,
        name,
        fingerprint,
        params
      )
}

export function keepsPreviousData(
  source: PanelSource,
  fingerprint: string,
  previousKey: readonly unknown[]
): boolean {
  if (source.kind === "version") {
    return (
      previousKey[1] === source.dashboardId && previousKey[3] === source.version
    )
  }
  return previousKey[1] === source.requestId && previousKey[4] === fingerprint
}
