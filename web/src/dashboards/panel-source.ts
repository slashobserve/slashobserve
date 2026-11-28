import {
  dashboardKeys,
  observationKeys,
  observeKeys,
  publicKeys,
  type PanelDataParams,
  type Selections,
  type VariableValuesParams,
} from "@/api/keys"
import { stableJson } from "@/lib/json"

export type PanelSource =
  | { kind: "version"; dashboardId: string; version: number }
  | { kind: "build"; requestId: string; saves: number }
  | { kind: "public"; token: string; version: number }
  | { kind: "block"; observationId: string; blockId: string; version: number }

export type VariableSource = Exclude<PanelSource, { kind: "block" }>

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
    case "block":
      return `block:${source.blockId}:${source.version}`
  }
}

export function loadRangeScope(
  source: PanelSource,
  range: { from?: string | undefined; to?: string | undefined }
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
    case "block":
      return `block:${source.blockId}`
  }
}

type PanelDataInput = {
  from?: string | undefined
  to?: string | undefined
  selections: Selections
  maxDataPoints?: number | undefined
}

function panelDataParams({
  from,
  to,
  selections,
  maxDataPoints,
}: PanelDataInput): PanelDataParams {
  return {
    ...(from === undefined ? {} : { from }),
    ...(to === undefined ? {} : { to }),
    selections,
    ...(maxDataPoints === undefined ? {} : { maxDataPoints }),
  }
}

export function panelDataKey(
  source: PanelSource,
  panelId: string,
  fingerprint: string,
  input: PanelDataInput
) {
  const params = panelDataParams(input)
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
    case "block":
      return observationKeys.blockData(source.blockId, source.version, params)
  }
}

export function variableValuesKey(
  source: VariableSource,
  name: string,
  fingerprint: string,
  {
    from,
    to,
    selections,
  }: {
    from?: string | undefined
    to?: string | undefined
    selections: Selections
  }
) {
  const params: VariableValuesParams = {
    ...(from === undefined ? {} : { from }),
    ...(to === undefined ? {} : { to }),
    selections,
  }
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
    case "block":
      return (
        previousKey[2] === source.blockId && previousKey[3] === source.version
      )
  }
}
