import { useMemo } from "react"

import type { WireFrame } from "@/dashboards/queries"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { createFieldResolver } from "@/panels/field-config"
import { prepareFrames, type DatasourceTypeOf } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

export function usePrepared(panel: DataPanel, frames: WireFrame[]) {
  const datasources = useDatasources()
  const typeOf = useMemo<DatasourceTypeOf>(
    () => (uid) => datasourceTypeOf(datasources.data, uid),
    [datasources.data]
  )
  const prepared = useMemo(
    () => prepareFrames(panel, frames, typeOf),
    [panel, frames, typeOf]
  )
  const fields = useMemo(() => createFieldResolver(panel, {}), [panel])
  const display = useMemo(
    () => displayWithDefaults(panel.type, panel.display),
    [panel]
  )
  return { frames: prepared, fields, display }
}
