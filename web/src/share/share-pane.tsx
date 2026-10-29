import { useId, useState } from "react"
import { Share2Icon } from "lucide-react"

import { CopyButton } from "@/components/copy-button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { searchFromState, type DashboardSearch } from "@/dashboards/search"
import { describeRange } from "@/lib/time"
import type { Dashboard } from "@/spec"

import {
  dashboardShareUrl,
  panelChoices,
  panelShareUrl,
  type ShareOptions,
} from "./share-links"
import { WhoHasAccess } from "./who-has-access"

type Props = {
  dashboardId: string
  spec: Dashboard
  search: DashboardSearch
  folder: { id: string; title: string; general: boolean }
  canManage: boolean
}

export function SharePane({
  dashboardId,
  spec,
  search,
  folder,
  canManage,
}: Props) {
  const linkId = useId()
  const panelId = useId()
  const whoId = useId()
  const [options, setOptions] = useState<ShareOptions>({
    time: true,
    variables: true,
  })
  const [chosen, setChosen] = useState<string>()

  const from = search.from ?? spec.time.from
  const to = search.to ?? spec.time.to
  const current = { ...searchFromState(search), from, to }
  const origin = window.location.origin
  const variableNames = Object.keys(search.vars)

  const choices = panelChoices(spec)
  const selected = choices.some((choice) => choice.id === chosen)
    ? chosen
    : choices[0]?.id

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-auto p-4">
      <section aria-labelledby={linkId} className="flex flex-col gap-2.5">
        <h3 id={linkId} className="text-[13px] font-semibold">
          Link to this dashboard
        </h3>
        <OptionCheckbox
          label="Include the time range"
          hint={describeRange(from, to)}
          checked={options.time}
          onChange={(time) => setOptions((prev) => ({ ...prev, time }))}
        />
        <OptionCheckbox
          label="Include variable values"
          hint={variableNames.join(", ")}
          checked={options.variables}
          onChange={(variables) =>
            setOptions((prev) => ({ ...prev, variables }))
          }
        />
        <LinkRow
          url={dashboardShareUrl(origin, dashboardId, current, options)}
        />
        <p className="text-xs text-muted-foreground">
          Links need sign-in. Someone without access to the folder sees
          &quot;You don&apos;t have access to this dashboard&quot;.
        </p>
      </section>
      {selected === undefined ? null : (
        <section aria-labelledby={panelId} className="flex flex-col gap-2">
          <h3 id={panelId} className="text-[13px] font-semibold">
            Link to one panel
          </h3>
          <div className="flex gap-1.5">
            <select
              aria-label="Panel"
              value={selected}
              onChange={(event) => setChosen(event.target.value)}
              className="h-8 min-w-0 flex-1 rounded-control border border-input bg-background px-2 text-[13px] outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring narrow:text-base"
            >
              {choices.map((choice) => (
                <option key={choice.id} value={choice.id}>
                  {choice.title}
                </option>
              ))}
            </select>
            <CopyButton
              text={panelShareUrl(
                origin,
                dashboardId,
                selected,
                current,
                options
              )}
              label="Copy link to the chosen panel"
              icon={false}
              variant="outline"
            >
              Copy link
            </CopyButton>
          </div>
        </section>
      )}
      <WhoHasAccess folder={folder} canManage={canManage} headingId={whoId} />
    </div>
  )
}

function OptionCheckbox({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <Label className="gap-2 text-[13px] font-normal">
      <Checkbox checked={checked} onCheckedChange={onChange} />
      <span>
        {label}
        {hint === "" ? null : (
          <span className="text-muted-foreground"> {hint}</span>
        )}
      </span>
    </Label>
  )
}

function LinkRow({ url }: { url: string }) {
  const inputId = useId()
  return (
    <div className="flex gap-1.5">
      <label htmlFor={inputId} className="sr-only">
        Dashboard link
      </label>
      <Input
        id={inputId}
        readOnly
        value={url}
        onFocus={(event) => event.currentTarget.select()}
        className="flex-1 font-mono text-xs text-muted-foreground"
      />
      <CopyButton text={url} label="Copy dashboard link" icon={false}>
        Copy link
      </CopyButton>
    </div>
  )
}

export function ShareToggle({
  pressed,
  disabled,
  onToggle,
}: {
  pressed: boolean
  disabled?: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      disabled={disabled}
      onClick={onToggle}
      className="flex h-[30px] items-center gap-1.5 rounded-control px-2 text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 aria-pressed:bg-muted"
    >
      <Share2Icon aria-hidden="true" className="size-4" />
      Share
    </button>
  )
}
