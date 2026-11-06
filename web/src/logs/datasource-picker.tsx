import { Label } from "@/components/ui/label"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"

const typeNames: Record<string, string> = {
  loki: "Loki",
  clickhouse: "ClickHouse",
  tempo: "Tempo",
}

const selectClasses =
  "min-h-8 rounded-control border border-input bg-background px-2 text-[13px] text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"

export function eligibleDatasources(
  list: readonly DatasourceStatus[] | undefined,
  types: readonly string[]
): DatasourceStatus[] {
  return (list ?? []).filter((datasource) => types.includes(datasource.type))
}

export function optionText(datasource: DatasourceStatus): string {
  const type = typeNames[datasource.type] ?? datasource.type
  const text =
    datasource.name.toLowerCase() === type.toLowerCase()
      ? type
      : `${datasource.name} (${type})`
  return datasource.healthy ? text : `${text} - unavailable`
}

export function ExploreDatasourcePicker({
  id,
  types,
  value,
  onChange,
  label,
}: {
  id: string
  types: readonly string[]
  value: string | undefined
  onChange: (uid: string) => void
  label: string
}) {
  const datasources = useDatasources()
  const eligible = eligibleDatasources(datasources.data, types)
  if (datasources.isPending) {
    return (
      <div className="flex items-center gap-2">
        <Label htmlFor={id}>{label}</Label>
        <select id={id} disabled className={selectClasses}>
          <option>Loading datasources</option>
        </select>
      </div>
    )
  }
  if (eligible.length === 0) return null
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id}>{label}</Label>
      <select
        id={id}
        value={value ?? ""}
        onChange={(event) => onChange(event.target.value)}
        className={selectClasses}
      >
        {value === undefined ? (
          <option value="">Pick a datasource</option>
        ) : null}
        {eligible.map((datasource) => (
          <option key={datasource.uid} value={datasource.uid}>
            {optionText(datasource)}
          </option>
        ))}
      </select>
    </div>
  )
}
