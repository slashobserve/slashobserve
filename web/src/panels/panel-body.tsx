import { Suspense, useMemo } from "react"

import type { Selections } from "@/api/keys"
import type { PanelDataResponse } from "@/dashboards/queries"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import {
  displayWithDefaults,
  resolveDatasourceRef,
  type DataPanel,
} from "@/spec"

import { createFieldResolver, type FieldResolver } from "./field-config"
import { dataRenderers, staticRenderers } from "./index"
import { PanelSkeleton } from "./panel-skeleton"
import { RendererBoundary } from "./renderer-boundary"
import type { SharedCursor } from "./shared-cursor"
import { prepareFrames, type DatasourceTypeOf } from "./transforms"
import type { ReusedFilter } from "./transforms/ad-hoc-filter"
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
    from,
    to,
    fields,
    fromMs: meta ? Date.parse(meta.from) : undefined,
    toMs: meta ? Date.parse(meta.to) : undefined,
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
  from: string | undefined
  to: string | undefined
  asTable?: boolean
  reusedFilter?: ReusedFilter
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
  from,
  to,
  asTable = false,
  reusedFilter,
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
  const frames = useMemo(
    () => prepareFrames(panel, data.frames, typeOf, reusedFilter),
    [panel, data.frames, typeOf, reusedFilter]
  )
  const ghostFrames = useMemo(
    () =>
      ghost
        ? prepareFrames(panel, ghost.frames, typeOf, reusedFilter)
        : undefined,
    [panel, ghost, typeOf, reusedFilter]
  )
  const fields = useMemo(() => createFieldResolver(panel, vars), [panel, vars])
  const display = useMemo(
    () => displayWithDefaults(panel.type, panel.display),
    [panel]
  )
  const links = useMemo(
    () => linkScopeFor({ vars, from, to, fields, meta: data.meta }),
    [vars, from, to, fields, data.meta]
  )
  const type = asTable ? "table" : panel.type
  const Renderer = dataRenderers[type]
  return (
    <RendererBoundary key={type}>
      <Suspense fallback={<PanelSkeleton />}>
        {Renderer ? (
          <Renderer
            panel={panel}
            frames={frames}
            fields={fields}
            display={display}
            meta={data.meta}
            width={width}
            height={height}
            stateScope={stateScope}
            cursor={cursor}
            onTimeRangeSelect={onTimeRangeSelect}
            links={links}
            ghostFrames={ghostFrames}
            exemplars={data.exemplars}
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
