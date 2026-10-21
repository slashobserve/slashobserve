import { useDatasources } from "@/datasources/queries"
import { cn } from "@/lib/utils"

const typeLabels: Record<string, string> = {
  prometheus: "Prometheus",
  clickhouse: "ClickHouse",
}

function typeLabel(type: string): string {
  return Object.hasOwn(typeLabels, type) ? typeLabels[type] : type
}

type Props = {
  id?: string
  value: string
  onChange: (uid: string) => void
  includePanelDefault?: string
  compact?: boolean
  ariaLabel?: string
  disabled?: boolean
}

export function DatasourceSelect({
  id,
  value,
  onChange,
  includePanelDefault,
  compact = false,
  ariaLabel,
  disabled = false,
}: Props) {
  const datasources = useDatasources()
  const list = datasources.data ?? []
  const known = list.some((datasource) => datasource.uid === value)
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
          ? "h-6 max-w-[220px] px-1.5 text-xs"
          : "h-7 px-2 text-[13px] narrow:text-base"
      )}
    >
      {includePanelDefault === undefined ? null : (
        <option value="">{`Panel datasource (${includePanelDefault})`}</option>
      )}
      {value !== "" && !known ? <option value={value}>{value}</option> : null}
      {list.map((datasource) => (
        <option key={datasource.uid} value={datasource.uid}>
          {`${datasource.uid} (${typeLabel(datasource.type)})`}
        </option>
      ))}
    </select>
  )
}
