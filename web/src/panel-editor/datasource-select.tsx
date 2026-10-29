import { useDatasources } from "@/datasources/queries"
import { cn } from "@/lib/utils"
import type { DatasourceVariable } from "@/spec"

const typeLabels: Record<string, string> = {
  prometheus: "Prometheus",
  clickhouse: "ClickHouse",
}

function typeLabel(type: string): string {
  return Object.hasOwn(typeLabels, type) ? typeLabels[type] : type
}

function variableRef(variable: DatasourceVariable): string {
  return "${" + variable.name + "}"
}

type Props = {
  id?: string
  value: string
  onChange: (uid: string) => void
  includePanelDefault?: string
  variables?: readonly DatasourceVariable[]
  compact?: boolean
  ariaLabel?: string
  disabled?: boolean
}

export function DatasourceSelect({
  id,
  value,
  onChange,
  includePanelDefault,
  variables = [],
  compact = false,
  ariaLabel,
  disabled = false,
}: Props) {
  const datasources = useDatasources()
  const list = datasources.data ?? []
  const known =
    list.some((datasource) => datasource.uid === value) ||
    variables.some((variable) => variableRef(variable) === value)
  return (
    <select
      id={id}
      aria-label={ariaLabel}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
      className={cn(
        "min-w-0 rounded-control border border-input bg-background font-normal text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
        compact
          ? "h-6 max-w-[220px] px-1.5 text-xs narrow:text-base"
          : "h-7 px-2 text-[13px] narrow:text-base"
      )}
    >
      {includePanelDefault === undefined ? null : (
        <option value="">{`Panel datasource (${includePanelDefault})`}</option>
      )}
      {variables.map((variable) => (
        <option key={variable.name} value={variableRef(variable)}>
          {`$${variable.name} (variable, ${typeLabel(variable.datasourceType)})`}
        </option>
      ))}
      {value !== "" && !known ? <option value={value}>{value}</option> : null}
      {list.map((datasource) => (
        <option key={datasource.uid} value={datasource.uid}>
          {`${datasource.uid} (${typeLabel(datasource.type)})`}
        </option>
      ))}
    </select>
  )
}
