import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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

const panelDefaultValue = "*"

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
  const items = [
    ...(includePanelDefault === undefined
      ? []
      : [
          {
            value: panelDefaultValue,
            label: `Panel datasource (${includePanelDefault})`,
          },
        ]),
    ...variables.map((variable) => ({
      value: variableRef(variable),
      label: `$${variable.name} (variable, ${typeLabel(variable.datasourceType)})`,
    })),
    ...(value !== "" && !known ? [{ value, label: value }] : []),
    ...list.map((datasource) => ({
      value: datasource.uid,
      label: `${datasource.uid} (${typeLabel(datasource.type)})`,
    })),
  ]
  const selected =
    value !== ""
      ? value
      : includePanelDefault === undefined
        ? null
        : panelDefaultValue
  return (
    <Select
      items={items}
      value={selected}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== null) onChange(next === panelDefaultValue ? "" : next)
      }}
    >
      <SelectTrigger
        id={id}
        aria-label={ariaLabel}
        size={compact ? "sm" : "default"}
        className={compact ? "max-w-[220px] data-[size=sm]:h-6" : undefined}
      >
        <SelectValue placeholder="Choose a datasource" />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
