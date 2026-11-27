import type { ComponentProps, ReactNode } from "react"

import type { Selections } from "@/api/keys"
import { describeRange, displayZone } from "@/lib/time"
import { ActionProvider } from "@/panels/actions/action-context"
import type { Dashboard } from "@/spec"

import type { PanelItem } from "../dashboard-grid"
import { pickerQuickRanges } from "../time-picker-settings"
import type { VariableState } from "../use-variables"
import { exportFileName } from "../export-spec"
import { ExportSheet } from "./export-sheet"
import { headerLines, variableLines } from "./header-text"
import { PrintGrid } from "./print-grid"

type Props = {
  spec: Dashboard
  folder: string | null
  version: number
  from: string
  to: string
  states: VariableState[]
  selections: Selections
  values: Record<string, string[] | undefined>
  actions: ComponentProps<typeof ActionProvider>["value"]
  renderPanel: (
    item: PanelItem,
    size: { width: number; height: number }
  ) => ReactNode
}

export function ExportView({
  spec,
  folder,
  version,
  from,
  to,
  states,
  selections,
  values,
  actions,
  renderPanel,
}: Props) {
  const lines = headerLines({
    folder,
    version,
    range: describeRange(
      from,
      to,
      pickerQuickRanges(spec),
      displayZone(undefined, spec.timezone)
    ),
    variables: variableLines(states),
  })
  return (
    <ExportSheet
      title={spec.title}
      context={lines.context}
      range={lines.range}
      variables={lines.variables}
      imageName={exportFileName(spec.title, version, "png")}
    >
      <ActionProvider value={actions}>
        <PrintGrid
          spec={spec}
          selections={selections}
          values={values}
          renderPanel={renderPanel}
        />
      </ActionProvider>
    </ExportSheet>
  )
}
