import { useId, useState } from "react"
import { Share2Icon } from "lucide-react"

import { CopyButton } from "@/components/copy-button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { searchFromState, type DashboardSearch } from "@/dashboards/search"
import { describeRange } from "@/lib/time"
import { PublicLinksSection } from "@/public/public-links-section"
import type { Dashboard } from "@/spec"

import {
  dashboardShareUrl,
  panelChoices,
  panelShareUrl,
  type ShareOptions,
} from "./share-links"
import { PublishSection } from "./publish-section"
import { WhoHasAccess } from "./who-has-access"

type Props = {
  dashboardId: string
  spec: Dashboard
  search: DashboardSearch
  folder: {
    id: string
    title: string
    general: boolean
    personal: boolean
  }
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
  const dashboard = { id: dashboardId, title: spec.title }
  const variableNames = Object.keys(search.vars)

  const choices = panelChoices(spec)
  const selected = choices.some((choice) => choice.id === chosen)
    ? chosen
    : choices[0]?.id
  const panelItems = choices.map((choice) => ({
    value: choice.id,
    label: choice.title,
  }))

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
        <LinkRow url={dashboardShareUrl(origin, dashboard, current, options)} />
        <p className="text-xs text-muted-foreground">
          {folder.personal
            ? "Only you can open this link until you publish the dashboard."
            : 'Links need sign-in. Someone without access to the folder sees "You don\'t have access to this dashboard".'}
        </p>
      </section>
      {selected === undefined ? null : (
        <section aria-labelledby={panelId} className="flex flex-col gap-2">
          <h3 id={panelId} className="text-[13px] font-semibold">
            Link to one panel
          </h3>
          <div className="flex gap-1.5">
            <Select
              items={panelItems}
              value={selected}
              onValueChange={(next) => {
                if (next !== null) setChosen(next)
              }}
            >
              <SelectTrigger aria-label="Panel" className="min-w-0 flex-1">
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false} align="start">
                {panelItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <CopyButton
              text={panelShareUrl(
                origin,
                dashboard,
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
      {folder.personal ? (
        <PublishSection dashboardId={dashboardId} headingId={whoId} />
      ) : (
        <WhoHasAccess folder={folder} canManage={canManage} headingId={whoId} />
      )}
      {canManage && !folder.personal ? (
        <PublicLinksSection
          dashboardId={dashboardId}
          variables={spec.variables}
          vars={search.vars}
        />
      ) : null}
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
