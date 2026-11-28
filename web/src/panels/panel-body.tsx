import { Suspense, useMemo } from "react"

import type { Annotation } from "@/annotations/queries"
import type { Selections } from "@/api/keys"
import type { PanelDataResponse } from "@/dashboards/queries"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { displayZone } from "@/lib/time"
import {
  displayWithDefaults,
  resolveDatasourceRef,
  type DataPanel,
  type Override,
} from "@/spec"

import { createFieldResolver, type FieldResolver } from "./field-config"
import { dataRenderers, staticRenderers } from "./index"
import { PanelSkeleton } from "./panel-skeleton"
import { RendererBoundary } from "./renderer-boundary"
import type { SharedCursor } from "./shared-cursor"
import {
  prepareFrames,
  type DatasourceTypeOf,
  type PreparedFrame,
} from "./transforms"
import type { ReusedFilter } from "./transforms/ad-hoc-filter"
import { useGazetteers } from "./transforms/use-gazetteers"
import type { PanelRendererProps, StaticPanelProps } from "./types"
import { Unsupported } from "./unsupported"

export function linkScopeFor({
  vars,
  from,
  to,
  fields,
  meta,
}: {
  vars: Selections
  from: string | undefined
  to: string | undefined
  fields: FieldResolver
  meta: PanelDataResponse["meta"] | undefined
}): PanelRendererProps["links"] {
  return {
    vars,
    ...(from === undefined ? {} : { from }),
    ...(to === undefined ? {} : { to }),
    fields,
    ...(meta
      ? { fromMs: Date.parse(meta.from), toMs: Date.parse(meta.to) }
      : {}),
  }
}

type Props = {
  panel: DataPanel
  data: PanelDataResponse
  ghost?: PanelDataResponse
  width: number
  height: number
  stateScope: string
  cursor: SharedCursor
  onTimeRangeSelect: PanelRendererProps["onTimeRangeSelect"]
  vars: Selections
  timezone: string | undefined
  from: string | undefined
  to: string | undefined
  asTable?: boolean
  reusedFilter?: ReusedFilter
  annotations?: readonly Annotation[]
  columnOverrides?: (
    frames: readonly PreparedFrame[],
    fields: FieldResolver
  ) => Override[]
}

export function PanelBody({
  panel,
  data,
  ghost,
  width,
  height,
  stateScope,
  cursor,
  onTimeRangeSelect,
  vars,
  timezone,
  from,
  to,
  asTable = false,
  reusedFilter,
  annotations,
  columnOverrides,
}: Props) {
  const datasources = useDatasources()
  const typeOf = useMemo<DatasourceTypeOf>(
    () => (ref) => {
      const uid = resolveDatasourceRef(ref, vars)
      return uid === undefined
        ? undefined
        : datasourceTypeOf(datasources.data, uid)
    },
    [datasources.data, vars]
  )
  const gazetteers = useGazetteers(panel.transform)
  const frames = useMemo(
    () =>
      prepareFrames(panel, data.frames, typeOf, reusedFilter, vars, gazetteers),
    [panel, data.frames, typeOf, reusedFilter, vars, gazetteers]
  )
  const ghostFrames = useMemo(
    () =>
      ghost
        ? prepareFrames(
            panel,
            ghost.frames,
            typeOf,
            reusedFilter,
            vars,
            gazetteers
          )
        : undefined,
    [panel, ghost, typeOf, reusedFilter, vars, gazetteers]
  )
  const given = useMemo(() => createFieldResolver(panel, vars), [panel, vars])
  const shown = useMemo(() => {
    const added = columnOverrides?.(frames, given) ?? []
    return added.length === 0
      ? panel
      : { ...panel, overrides: [...added, ...(panel.overrides ?? [])] }
  }, [panel, frames, given, columnOverrides])
  const fields = useMemo(
    () => (shown === panel ? given : createFieldResolver(shown, vars)),
    [shown, panel, given, vars]
  )
  const display = useMemo(
    () => displayWithDefaults(panel.type, panel.display),
    [panel]
  )
  const links = useMemo(
    () => linkScopeFor({ vars, from, to, fields, meta: data.meta }),
    [vars, from, to, fields, data.meta]
  )
  const zone = displayZone(panel.timezone, timezone)
  const type = asTable ? "table" : panel.type
  const Renderer = dataRenderers[type]
  return (
    <RendererBoundary key={type}>
      <Suspense fallback={<PanelSkeleton />}>
        {Renderer ? (
          <Renderer
            panel={shown}
            frames={frames}
            fields={fields}
            display={display}
            zone={zone}
            meta={data.meta}
            width={width}
            height={height}
            stateScope={stateScope}
            cursor={cursor}
            onTimeRangeSelect={onTimeRangeSelect}
            links={links}
            ghostFrames={ghostFrames}
            exemplars={data.exemplars}
            annotations={annotations}
          />
        ) : (
          <Unsupported />
        )}
      </Suspense>
    </RendererBoundary>
  )
}

export function StaticPanelBody({
  panel,
  width,
  height,
  vars,
  from,
  to,
}: StaticPanelProps) {
  const Renderer = staticRenderers[panel.type]
  return (
    <RendererBoundary>
      <Suspense fallback={<PanelSkeleton />}>
        {Renderer ? (
          <Renderer
            panel={panel}
            width={width}
            height={height}
            vars={vars}
            from={from}
            to={to}
          />
        ) : (
          <Unsupported />
        )}
      </Suspense>
    </RendererBoundary>
  )
}
