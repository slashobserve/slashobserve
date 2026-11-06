import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"

const typeNames: Record<string, string> = {
  loki: "Loki",
  clickhouse: "ClickHouse",
  tempo: "Tempo",
  elasticsearch: "Elasticsearch",
  opensearch: "OpenSearch",
}

export function typeName(type: string): string {
  return typeNames[type] ?? type
}

export function eligibleDatasources(
  list: readonly DatasourceStatus[] | undefined,
  types: readonly string[]
): DatasourceStatus[] {
  return (list ?? []).filter((datasource) => types.includes(datasource.type))
}

export function optionText(datasource: DatasourceStatus): string {
  const type = typeName(datasource.type)
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
  const loading = datasources.isPending
  const eligible = eligibleDatasources(datasources.data, types)
  if (!loading && eligible.length === 0) return null
  const items = eligible.map((datasource) => ({
    value: datasource.uid,
    label: optionText(datasource),
  }))
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Select
        items={items}
        value={value ?? null}
        disabled={loading}
        onValueChange={(next) => {
          if (next !== null) onChange(next)
        }}
      >
        <SelectTrigger id={id}>
          <SelectValue
            placeholder={loading ? "Loading datasources" : "Pick a datasource"}
          />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
