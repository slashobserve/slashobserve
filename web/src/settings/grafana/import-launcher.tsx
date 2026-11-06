import { useId, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon, PuzzleIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { datasourceKindLabel } from "@/import/report"

import { grafanaProblemText } from "./connection-problems"
import {
  mappingToSend,
  openMatches,
  prefilledMapping,
  supported,
  unsupported,
  type GrafanaDatasource,
  type Picks,
} from "./mapping"
import {
  grafanaDatasourcesQuery,
  grafanaFoldersQuery,
  useStartImport,
  type GrafanaConnection,
  type GrafanaFolder,
  type GrafanaImport,
} from "./queries"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"

type Props = {
  connection: GrafanaConnection
  onCancel: () => void
  onStarted: (sync: GrafanaImport) => void
}

type Scope = "all" | "some"

function plural(n: number, one: string, many: string) {
  return `${n} ${n === 1 ? one : many}`
}

export function ImportLauncher({ connection, onCancel, onStarted }: Props) {
  const folders = useQuery(grafanaFoldersQuery(connection.id))
  const datasources = useQuery(grafanaDatasourcesQuery(connection.id))
  const start = useStartImport()
  const [scope, setScope] = useState<Scope>("all")
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set())
  const [changes, setChanges] = useState<Picks>({})
  const picks = useMemo(
    () => ({ ...prefilledMapping(datasources.data ?? []), ...changes }),
    [datasources.data, changes]
  )
  const title = `Import from ${connection.name}`
  const hint = "Choose what to bring over and check the datasource matches."

  if (folders.isError || datasources.isError) {
    const failed = folders.isError ? folders : datasources
    return (
      <SettingsCard title={title} hint={hint}>
        <SettingsProblem
          title="Could not read this Grafana"
          detail={grafanaProblemText(failed.error)}
          retrying={failed.isFetching}
          onRetry={() => void failed.refetch()}
        />
      </SettingsCard>
    )
  }
  if (folders.data === undefined || datasources.data === undefined) {
    return (
      <SettingsCard title={title} hint={hint}>
        <div className="flex flex-col gap-2 p-4" aria-busy="true">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-4 w-64" />
        </div>
      </SettingsCard>
    )
  }

  const list = datasources.data
  const open = openMatches(list, picks)
  const nothingChosen = scope === "some" && chosen.size === 0

  function toggle(uid: string, on: boolean) {
    setChosen((current) => {
      const next = new Set(current)
      if (on) next.add(uid)
      else next.delete(uid)
      return next
    })
  }

  function begin() {
    if (nothingChosen || start.isPending) return
    const mapping = mappingToSend(list, picks)
    start.mutate(
      {
        connectionId: connection.id,
        body: {
          folders: scope === "all" ? "all" : [...chosen],
          ...(Object.keys(mapping).length === 0 ? {} : { mapping }),
        },
      },
      { onSuccess: onStarted }
    )
  }

  return (
    <SettingsCard title={title} hint={hint}>
      <FolderChoice
        folders={folders.data}
        scope={scope}
        chosen={chosen}
        onScope={setScope}
        onToggle={toggle}
      />
      <DatasourceChoice
        datasources={list}
        picks={picks}
        onPick={(uid, pick) =>
          setChanges((current) => ({ ...current, [uid]: pick }))
        }
      />
      <div className="flex flex-col gap-3 px-4 py-3">
        {open > 0 ? (
          <StatusLabel tone="warning">
            {plural(open, "datasource has", "datasources have")} no match;
            dashboards that use {open === 1 ? "it" : "them"} fail until mapped.
          </StatusLabel>
        ) : null}
        {start.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {grafanaProblemText(start.error)}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={nothingChosen || start.isPending}
            focusableWhenDisabled
            aria-busy={start.isPending}
            onClick={begin}
          >
            {start.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Start import
          </Button>
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </div>
    </SettingsCard>
  )
}

function FolderChoice({
  folders,
  scope,
  chosen,
  onScope,
  onToggle,
}: {
  folders: GrafanaFolder[]
  scope: Scope
  chosen: ReadonlySet<string>
  onScope: (scope: Scope) => void
  onToggle: (uid: string, on: boolean) => void
}) {
  const legendId = useId()
  const total = folders.reduce((sum, folder) => sum + folder.dashboards, 0)
  return (
    <fieldset
      aria-labelledby={legendId}
      className="flex flex-col gap-2 border-b px-4 py-3"
    >
      <legend id={legendId} className="mb-1 text-[13px] font-medium">
        Folders
      </legend>
      <RadioGroup
        value={scope}
        onValueChange={(next) => onScope(next as Scope)}
      >
        <label className="flex items-center gap-2 text-[13px]">
          <RadioGroupItem value="all" />
          All folders ({plural(total, "dashboard", "dashboards")})
        </label>
        <label className="flex items-center gap-2 text-[13px]">
          <RadioGroupItem value="some" />
          Choose folders
        </label>
      </RadioGroup>
      {scope === "some" ? (
        <div className="flex flex-col gap-1.5 pl-6">
          <p className="text-xs text-muted-foreground">
            A folder brings its subfolders with it.
          </p>
          <ul className="flex flex-col gap-1.5">
            {folders.map((folder) => (
              <li key={folder.uid}>
                <label className="flex items-center gap-2 text-[13px]">
                  <Checkbox
                    checked={chosen.has(folder.uid)}
                    onCheckedChange={(on) => onToggle(folder.uid, on)}
                  />
                  <span className="wrap-anywhere">{folder.title}</span>
                  <span className="text-muted-foreground">
                    ({folder.dashboards})
                  </span>
                </label>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </fieldset>
  )
}

function Match({
  datasource,
  value,
  onPick,
}: {
  datasource: GrafanaDatasource
  value: string | null
  onPick: (uid: string, pick: string) => void
}) {
  const kind = datasourceKindLabel(datasource.kind ?? "")
  if (datasource.candidates.length === 0) {
    return (
      <span className="inline-flex items-center gap-1.5 text-muted-foreground">
        <PuzzleIcon aria-hidden="true" className="size-3.5 flex-none" />
        No {kind} datasource configured, shown as placeholders
      </span>
    )
  }
  const items = datasource.candidates.map((uid) => ({
    value: uid,
    label: `${uid} (${kind})`,
  }))
  return (
    <Select
      items={items}
      value={value}
      onValueChange={(pick) => pick && onPick(datasource.uid, pick)}
    >
      <SelectTrigger
        aria-label={`Datasource for ${datasource.name}`}
        className="w-full max-w-60"
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

function DatasourceChoice({
  datasources,
  picks,
  onPick,
}: {
  datasources: GrafanaDatasource[]
  picks: Picks
  onPick: (uid: string, pick: string) => void
}) {
  const headingId = useId()
  const matchable = supported(datasources)
  const others = unsupported(datasources).length
  return (
    <div className="flex flex-col gap-2 border-b px-4 py-3">
      <h3 id={headingId} className="text-[13px] font-medium">
        Datasources
      </h3>
      {matchable.length > 0 ? (
        <TableCard labelledBy={headingId} minWidth={520}>
          <thead>
            <tr>
              <HeaderCell>In Grafana</HeaderCell>
              <HeaderCell>Type</HeaderCell>
              <HeaderCell>Use in SlashObserve</HeaderCell>
            </tr>
          </thead>
          <tbody>
            {matchable.map((datasource) => (
              <tr key={datasource.uid}>
                <Cell className="wrap-anywhere">{datasource.name}</Cell>
                <Cell className="font-mono text-xs">{datasource.type}</Cell>
                <Cell>
                  <Match
                    datasource={datasource}
                    value={picks[datasource.uid] ?? null}
                    onPick={onPick}
                  />
                </Cell>
              </tr>
            ))}
          </tbody>
        </TableCard>
      ) : (
        <p className="text-[13px] text-muted-foreground">
          This Grafana has no datasource of a type SlashObserve queries.
        </p>
      )}
      {others > 0 ? (
        <p className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <PuzzleIcon aria-hidden="true" className="size-3.5 flex-none" />
          {plural(
            others,
            "datasource has a type",
            "datasources have types"
          )}{" "}
          SlashObserve does not query; their panels import as placeholders.
        </p>
      ) : null}
    </div>
  )
}
