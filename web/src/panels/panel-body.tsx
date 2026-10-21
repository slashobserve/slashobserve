import { Suspense, useMemo } from "react"

import type { Selections } from "@/api/keys"
import type { PanelDataResponse } from "@/dashboards/queries"
import { displayWithDefaults, type DataPanel } from "@/spec"

import { createFieldResolver, type FieldResolver } from "./field-config"
import { dataRenderers, staticRenderers } from "./index"
import { PanelSkeleton } from "./panel-skeleton"
import { RendererBoundary } from "./renderer-boundary"
import { prepareFrames } from "./transforms"
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
  syncKey: string
  onTimeRangeSelect: PanelRendererProps["onTimeRangeSelect"]
  vars: Selections
  from: string | undefined
  to: string | undefined
  asTable?: boolean
}

export function PanelBody({
  panel,
  data,
  ghost,
  width,
  height,
  syncKey,
  onTimeRangeSelect,
  vars,
  from,
  to,
  asTable = false,
}: Props) {
  const frames = useMemo(
    () => prepareFrames(panel, data.frames),
    [panel, data.frames]
  )
  const ghostFrames = useMemo(
    () => (ghost ? prepareFrames(panel, ghost.frames) : undefined),
    [panel, ghost]
  )
  const fields = useMemo(() => createFieldResolver(panel), [panel])
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
            syncKey={syncKey}
            onTimeRangeSelect={onTimeRangeSelect}
            links={links}
            ghostFrames={ghostFrames}
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
}: StaticPanelProps) {
  const Renderer = staticRenderers[panel.type]
  return (
    <RendererBoundary>
      <Suspense fallback={<PanelSkeleton />}>
        {Renderer ? (
          <Renderer panel={panel} width={width} height={height} vars={vars} />
        ) : (
          <Unsupported />
        )}
      </Suspense>
    </RendererBoundary>
  )
}
