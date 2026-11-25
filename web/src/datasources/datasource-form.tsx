import { useId, useState, type ReactNode, type RefObject } from "react"
import { PlusIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

import {
  addressHint,
  audienceChoices,
  datasourceTypes,
  fieldsOf,
  isAudience,
  toggleClear,
  type FormValues,
  type Problems,
  type SavedCredentials,
} from "./draft"
import { SourceKindSelect } from "./source-kind-select"

type Props = {
  mode: "create" | "edit"
  values: FormValues
  problems: Problems
  saved: SavedCredentials
  onChange: (change: Partial<FormValues>) => void
  onPickMcp: () => void
  firstInputRef: RefObject<HTMLElement | null>
}

function Field({
  id,
  label,
  hint,
  error,
  children,
}: {
  id: string
  label: string
  hint?: string
  error?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-[13px] font-medium">
        {label}
      </Label>
      {children}
      {error === undefined ? null : (
        <p id={`${id}-error`} className="text-xs text-destructive">
          {error}
        </p>
      )}
      {hint === undefined ? null : (
        <p id={`${id}-hint`} className="text-xs text-muted-foreground">
          {hint}
        </p>
      )}
    </div>
  )
}

function describedBy(id: string, error?: string, hint?: string) {
  const ids = [
    error === undefined ? null : `${id}-error`,
    hint === undefined ? null : `${id}-hint`,
  ].filter((entry) => entry !== null)
  return ids.length === 0 ? undefined : ids.join(" ")
}

function TextField({
  baseId,
  name,
  label,
  value,
  onValue,
  problems,
  path,
  placeholder,
  hint,
  mono = true,
  inputRef,
  disabled,
}: {
  baseId: string
  name: string
  label: string
  value: string
  onValue: (value: string) => void
  problems: Problems
  path: string
  placeholder?: string
  hint?: string
  mono?: boolean
  inputRef?: RefObject<HTMLElement | null>
  disabled?: boolean
}) {
  const id = `${baseId}-${name}`
  const error = problems[path]
  return (
    <Field id={id} label={label} hint={hint} error={error}>
      <Input
        id={id}
        ref={inputRef as RefObject<HTMLInputElement> | undefined}
        autoComplete="off"
        spellCheck={false}
        disabled={disabled}
        placeholder={placeholder}
        className={cn("narrow:h-11", mono && "font-mono")}
        value={value}
        aria-invalid={error !== undefined}
        aria-describedby={describedBy(id, error, hint)}
        onChange={(event) => onValue(event.target.value)}
      />
    </Field>
  )
}

function CredentialField({
  baseId,
  name,
  label,
  isSet,
  value,
  cleared,
  error,
  onValue,
  onToggleClear,
}: {
  baseId: string
  name: string
  label: string
  isSet: boolean
  value: string
  cleared: boolean
  error?: string
  onValue: (value: string) => void
  onToggleClear: () => void
}) {
  const id = `${baseId}-${name}`
  const hint = cleared
    ? "Will be removed when you save."
    : isSet
      ? "Set. Leave empty to keep it, or type a new value to replace it."
      : "Not set."
  return (
    <Field id={id} label={label} hint={hint} error={error}>
      <div className="flex gap-1.5">
        <Input
          id={id}
          type="password"
          autoComplete="new-password"
          spellCheck={false}
          disabled={cleared}
          placeholder={isSet ? "Unchanged" : ""}
          className="font-mono narrow:h-11"
          value={value}
          aria-invalid={error !== undefined}
          aria-describedby={describedBy(id, error, hint)}
          onChange={(event) => onValue(event.target.value)}
        />
        {isSet ? (
          <Button
            type="button"
            variant="outline"
            className="narrow:h-11"
            aria-pressed={cleared}
            aria-label={`${cleared ? "Keep" : "Clear"} ${label.toLowerCase()}`}
            onClick={onToggleClear}
          >
            {cleared ? "Keep" : "Clear"}
          </Button>
        ) : null}
      </div>
    </Field>
  )
}

export function DatasourceForm({
  mode,
  values,
  problems,
  saved,
  onChange,
  onPickMcp,
  firstInputRef,
}: Props) {
  const baseId = useId()
  const [advanced, setAdvanced] = useState(false)
  const fields = fieldsOf(values.type)
  const typeId = `${baseId}-type`

  function setHeader(
    index: number,
    change: Partial<{ name: string; value: string }>
  ) {
    onChange({
      headers: values.headers.map((row, at) =>
        at === index ? { ...row, ...change } : row
      ),
    })
  }

  return (
    <div className="flex flex-col gap-4">
      {mode === "create" ? (
        <Field id={typeId} label="Type" error={problems.type}>
          <SourceKindSelect
            id={typeId}
            value={values.type}
            triggerRef={firstInputRef}
            invalid={problems.type !== undefined}
            describedBy={describedBy(typeId, problems.type)}
            onChange={(kind) => {
              if (kind === "mcp") onPickMcp()
              else onChange({ type: kind })
            }}
          />
        </Field>
      ) : (
        <div className="flex flex-col gap-0.5 text-[13px]">
          <span className="font-medium">
            {datasourceTypes.find((type) => type.value === values.type)?.label}
          </span>
          <span className="font-mono text-xs text-muted-foreground">
            {values.uid}
          </span>
        </div>
      )}
      {mode === "create" ? (
        <TextField
          baseId={baseId}
          name="uid"
          label="Id"
          value={values.uid}
          onValue={(uid) => onChange({ uid })}
          problems={problems}
          path="uid"
          placeholder="prometheus-prod"
          hint="Dashboards refer to the datasource by this id. It cannot be changed later."
        />
      ) : null}
      <TextField
        baseId={baseId}
        name="name"
        label="Name"
        value={values.name}
        onValue={(name) => onChange({ name })}
        problems={problems}
        path="name"
        placeholder="Production Prometheus"
        hint="Optional. The id is shown when empty."
        mono={false}
        inputRef={mode === "edit" ? firstInputRef : undefined}
      />
      <TextField
        baseId={baseId}
        name="address"
        label="Address"
        value={values.address}
        onValue={(address) => onChange({ address })}
        problems={problems}
        path="address"
        placeholder={addressHint(values.type)}
      />
      {fields.database ? (
        <>
          <TextField
            baseId={baseId}
            name="database"
            label="Database"
            value={values.database}
            onValue={(database) => onChange({ database })}
            problems={problems}
            path="database"
            placeholder="otel"
          />
          <label className="flex min-h-7 items-center gap-2 text-[13px] narrow:min-h-11">
            <Checkbox
              checked={values.tls}
              onCheckedChange={(tls) => onChange({ tls })}
            />
            Connect over TLS
          </label>
        </>
      ) : null}
      {fields.tenantId ? (
        <TextField
          baseId={baseId}
          name="tenant"
          label="Tenant"
          value={values.tenantId}
          onValue={(tenantId) => onChange({ tenantId })}
          problems={problems}
          path="tenant_id"
          hint="Sent as X-Scope-OrgID. Leave empty without multi-tenancy."
        />
      ) : null}
      {fields.scrapeInterval ? (
        <TextField
          baseId={baseId}
          name="scrape"
          label="Scrape interval"
          value={values.scrapeInterval}
          onValue={(scrapeInterval) => onChange({ scrapeInterval })}
          problems={problems}
          path="scrape_interval"
          placeholder="15s"
        />
      ) : null}
      {fields.index ? (
        <>
          <TextField
            baseId={baseId}
            name="index"
            label="Index pattern"
            value={values.index}
            onValue={(index) => onChange({ index })}
            problems={problems}
            path="index"
            placeholder="logs-*"
          />
          <TextField
            baseId={baseId}
            name="time-field"
            label="Time field"
            value={values.timeField}
            onValue={(timeField) => onChange({ timeField })}
            problems={problems}
            path="time_field"
            placeholder="@timestamp"
          />
        </>
      ) : null}

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 text-[13px] font-semibold">Audience</legend>
        <RadioGroup
          value={values.audience}
          onValueChange={(pick) => {
            if (isAudience(pick)) onChange({ audience: pick })
          }}
        >
          {audienceChoices.map((choice) => (
            <label
              key={choice.value}
              className="flex min-h-7 items-center gap-2 text-[13px] narrow:min-h-11"
            >
              <RadioGroupItem value={choice.value} />
              {choice.label}
            </label>
          ))}
        </RadioGroup>
        <p className="text-xs text-muted-foreground">
          Only Admins means only Admins can query it, and dashboards, searches
          and observations that read it must live in a folder set to Only
          Admins.
        </p>
      </fieldset>

      <fieldset className="flex flex-col gap-4">
        <legend className="mb-2 text-[13px] font-semibold">Credentials</legend>
        <p className="-mt-1 text-xs text-muted-foreground">
          Stored encrypted and never shown again.
        </p>
        <TextField
          baseId={baseId}
          name="username"
          label="Username"
          value={values.username}
          onValue={(username) => onChange({ username })}
          problems={problems}
          path="username"
        />
        <CredentialField
          baseId={baseId}
          name="password"
          label="Password"
          isSet={saved.password}
          value={values.password}
          cleared={values.clear.includes("password")}
          error={problems["secrets.password"]}
          onValue={(password) => onChange({ password })}
          onToggleClear={() =>
            onChange({
              clear: toggleClear(values.clear, "password"),
              password: "",
            })
          }
        />
        {fields.bearerToken ? (
          <CredentialField
            baseId={baseId}
            name="bearer"
            label="Bearer token"
            isSet={saved.bearerToken}
            value={values.bearerToken}
            cleared={values.clear.includes("bearer_token")}
            error={problems["secrets.bearer_token"]}
            onValue={(bearerToken) => onChange({ bearerToken })}
            onToggleClear={() =>
              onChange({
                clear: toggleClear(values.clear, "bearer_token"),
                bearerToken: "",
              })
            }
          />
        ) : null}
        {fields.apiKey ? (
          <CredentialField
            baseId={baseId}
            name="api-key"
            label="API key"
            isSet={saved.apiKey}
            value={values.apiKey}
            cleared={values.clear.includes("api_key")}
            error={problems["secrets.api_key"]}
            onValue={(apiKey) => onChange({ apiKey })}
            onToggleClear={() =>
              onChange({
                clear: toggleClear(values.clear, "api_key"),
                apiKey: "",
              })
            }
          />
        ) : null}
        {fields.headers ? (
          <div className="flex flex-col gap-2">
            <span className="text-[13px] font-medium">Request headers</span>
            <p className="text-xs text-muted-foreground">
              Values are credentials: stored encrypted and never shown again.
            </p>
            {problems["secrets.headers"] === undefined ? null : (
              <p className="text-xs text-destructive">
                {problems["secrets.headers"]}
              </p>
            )}
            {saved.headers.map((name) => {
              const cleared = values.clear.includes(`header:${name}`)
              return (
                <div key={name} className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      "min-w-0 flex-1 truncate font-mono text-xs",
                      cleared && "line-through"
                    )}
                  >
                    {name}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {cleared ? "Removed on save" : "Set"}
                  </span>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="narrow:h-11"
                    aria-pressed={cleared}
                    aria-label={`${cleared ? "Keep" : "Clear"} header ${name}`}
                    onClick={() =>
                      onChange({
                        clear: toggleClear(values.clear, `header:${name}`),
                      })
                    }
                  >
                    {cleared ? "Keep" : "Clear"}
                  </Button>
                </div>
              )
            })}
            {values.headers.map((row, index) => {
              const error = problems[`headers.${index}`]
              return (
                <div key={index} className="flex flex-col gap-1">
                  <div className="flex gap-1.5">
                    <Input
                      aria-label={`Header ${index + 1} name`}
                      autoComplete="off"
                      spellCheck={false}
                      placeholder="X-Org-Id"
                      className="font-mono narrow:h-11"
                      value={row.name}
                      aria-invalid={error !== undefined}
                      onChange={(event) =>
                        setHeader(index, { name: event.target.value })
                      }
                    />
                    <Input
                      aria-label={`Header ${index + 1} value`}
                      type="password"
                      autoComplete="new-password"
                      spellCheck={false}
                      placeholder="Value"
                      className="font-mono narrow:h-11"
                      value={row.value}
                      aria-invalid={error !== undefined}
                      onChange={(event) =>
                        setHeader(index, { value: event.target.value })
                      }
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      className="narrow:size-11"
                      aria-label={`Remove header ${index + 1}`}
                      onClick={() =>
                        onChange({
                          headers: values.headers.filter(
                            (_, at) => at !== index
                          ),
                        })
                      }
                    >
                      <XIcon aria-hidden="true" />
                    </Button>
                  </div>
                  {error === undefined ? null : (
                    <p className="text-xs text-destructive">{error}</p>
                  )}
                </div>
              )
            })}
            <div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="narrow:h-11"
                onClick={() =>
                  onChange({
                    headers: [...values.headers, { name: "", value: "" }],
                  })
                }
              >
                <PlusIcon aria-hidden="true" />
                Add header
              </Button>
            </div>
          </div>
        ) : null}
      </fieldset>

      <div className="flex flex-col gap-4">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="w-fit px-0 font-semibold hover:bg-transparent"
          aria-expanded={advanced}
          aria-controls={`${baseId}-advanced`}
          onClick={() => setAdvanced((open) => !open)}
        >
          {advanced ? "Hide advanced settings" : "Show advanced settings"}
        </Button>
        <div
          id={`${baseId}-advanced`}
          hidden={!advanced}
          className="flex flex-col gap-4"
        >
          <TextField
            baseId={baseId}
            name="timeout"
            label="Query timeout"
            value={values.timeout}
            onValue={(timeout) => onChange({ timeout })}
            problems={problems}
            path="limits.timeout"
            placeholder="30s"
            hint="Empty uses the default."
          />
          <TextField
            baseId={baseId}
            name="max-series"
            label="Max series"
            value={values.maxSeries}
            onValue={(maxSeries) => onChange({ maxSeries })}
            problems={problems}
            path="limits.max_series"
          />
          <TextField
            baseId={baseId}
            name="max-points"
            label="Max points"
            value={values.maxPoints}
            onValue={(maxPoints) => onChange({ maxPoints })}
            problems={problems}
            path="limits.max_points"
          />
          <TextField
            baseId={baseId}
            name="max-rows"
            label="Max rows"
            value={values.maxRows}
            onValue={(maxRows) => onChange({ maxRows })}
            problems={problems}
            path="limits.max_rows"
          />
          <TextField
            baseId={baseId}
            name="max-concurrent"
            label="Max concurrent queries"
            value={values.maxConcurrent}
            onValue={(maxConcurrent) => onChange({ maxConcurrent })}
            problems={problems}
            path="limits.max_concurrent_queries"
            hint="Across all servers. Empty uses the default of 32."
          />
          <TextField
            baseId={baseId}
            name="grafana"
            label="Grafana datasource ids"
            value={values.grafanaUids}
            onValue={(grafanaUids) => onChange({ grafanaUids })}
            problems={problems}
            path="grafana_uids"
            hint="Comma separated. Dashboards imported from Grafana that use these ids use this datasource."
          />
          {fields.index ? (
            <>
              <QueryLanguage
                values={values}
                onChange={onChange}
                baseId={baseId}
              />
              {(
                [
                  [
                    "field-message",
                    "Message field",
                    "fieldMessage",
                    "fields.message",
                  ],
                  ["field-level", "Level field", "fieldLevel", "fields.level"],
                  [
                    "field-service",
                    "Service field",
                    "fieldService",
                    "fields.service",
                  ],
                  [
                    "field-trace",
                    "Trace id field",
                    "fieldTraceId",
                    "fields.trace_id",
                  ],
                  [
                    "field-span",
                    "Span id field",
                    "fieldSpanId",
                    "fields.span_id",
                  ],
                ] as const
              ).map(([name, label, key, path]) => (
                <TextField
                  key={name}
                  baseId={baseId}
                  name={name}
                  label={label}
                  value={values[key]}
                  onValue={(value) => onChange({ [key]: value })}
                  problems={problems}
                  path={path}
                />
              ))}
            </>
          ) : null}
        </div>
      </div>
    </div>
  )
}

const languages = [
  { value: "", label: "Default" },
  { value: "kql", label: "KQL" },
  { value: "lucene", label: "Lucene" },
]

function QueryLanguage({
  values,
  onChange,
  baseId,
}: {
  values: FormValues
  onChange: (change: Partial<FormValues>) => void
  baseId: string
}) {
  const id = `${baseId}-language`
  return (
    <Field id={id} label="Query language">
      <Select
        items={languages}
        value={values.queryLanguage}
        onValueChange={(pick) => {
          if (pick === "" || pick === "kql" || pick === "lucene") {
            onChange({ queryLanguage: pick })
          }
        }}
      >
        <SelectTrigger id={id} className="w-full narrow:h-11">
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {languages.map((language) => (
            <SelectItem key={language.value} value={language.value}>
              {language.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}
