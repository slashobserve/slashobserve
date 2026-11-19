import {
  dashboardKeys,
  observeKeys,
  publicKeys,
  type PanelDataParams,
  type VariableValuesParams,
} from "@/api/keys"
import { stableJson } from "@/lib/json"

export type PanelSource =
  | { kind: "version"; dashboardId: string; version: number }
  | { kind: "build"; requestId: string; saves: number }
  | { kind: "public"; token: string; version: number }

export function panelFingerprint(panel: unknown): string {
  return stableJson(panel)
}

export function sourceScope(source: PanelSource): string {
  switch (source.kind) {
    case "version":
      return `${source.dashboardId}:${source.version}`
    case "build":
      return `build:${source.requestId}`
    case "public":
      return `public:${source.token}:${source.version}`
  }
}

export function loadRangeScope(
  source: PanelSource,
  range: { from?: string; to?: string }
): string {
  return source.kind === "build"
    ? `${sourceScope(source)}:${range.from ?? ""}:${range.to ?? ""}`
    : sourceScope(source)
}

export function stateScope(source: PanelSource): string {
  switch (source.kind) {
    case "version":
      return source.dashboardId
    case "build":
      return `build:${source.requestId}`
    case "public":
      return `public:${source.token}`
  }
}

export function panelDataKey(
  source: PanelSource,
  panelId: string,
  fingerprint: string,
  params: PanelDataParams
) {
  switch (source.kind) {
    case "version":
      return dashboardKeys.panelData(
        source.dashboardId,
        source.version,
        panelId,
        params
      )
    case "build":
      return observeKeys.buildPanelData(
        source.requestId,
        panelId,
        fingerprint,
        params
      )
    case "public":
      return publicKeys.panelData(source.token, source.version, panelId, params)
  }
}

export function variableValuesKey(
  source: PanelSource,
  name: string,
  fingerprint: string,
  params: VariableValuesParams
) {
  switch (source.kind) {
    case "version":
      return dashboardKeys.variableValues(
        source.dashboardId,
        source.version,
        name,
        params
      )
    case "build":
      return observeKeys.buildVariableValues(
        source.requestId,
        name,
        fingerprint,
        params
      )
    case "public":
      return publicKeys.variableValues(
        source.token,
        source.version,
        name,
        params
      )
  }
}

export function keepsPreviousData(
  source: PanelSource,
  fingerprint: string,
  previousKey: readonly unknown[]
): boolean {
  switch (source.kind) {
    case "version":
      return (
        previousKey[1] === source.dashboardId &&
        previousKey[3] === source.version
      )
    case "build":
      return (
        previousKey[1] === source.requestId && previousKey[4] === fingerprint
      )
    case "public":
      return (
        previousKey[1] === source.token && previousKey[3] === source.version
      )
  }
}
