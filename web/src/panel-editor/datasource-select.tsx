import { NativeSelect } from "@/components/native-select"
import { useDatasources } from "@/datasources/queries"
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
    <NativeSelect
      id={id}
      aria-label={ariaLabel}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
      size={compact ? "sm" : "default"}
      className={compact ? "h-6 max-w-[220px]" : undefined}
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
    </NativeSelect>
  )
}
