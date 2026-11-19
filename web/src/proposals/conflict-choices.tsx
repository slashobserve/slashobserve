import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"

import { conflictText } from "./proposal-view"
import type { MergeConflict } from "./queries"

export type Side = "ours" | "theirs"

const sides: { value: Side; label: string }[] = [
  { value: "ours", label: "Keep the dashboard's" },
  { value: "theirs", label: "Take this proposal's" },
]

type Props = {
  conflict: MergeConflict
  label: string
  dashboardTitle?: string | null
  proposalTitle?: string | null
  value: Side | undefined
  onChange: (side: Side) => void
  disabled?: boolean
}

export function ConflictChoices({
  conflict,
  label,
  dashboardTitle = null,
  proposalTitle = null,
  value,
  onChange,
  disabled = false,
}: Props) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[13px] wrap-anywhere">
        {conflictText(conflict, label)}
      </p>
      {dashboardTitle === null || proposalTitle === null ? null : (
        <dl className="grid grid-cols-[auto_1fr] gap-x-2 text-xs text-muted-foreground">
          <dt>Dashboard:</dt>
          <dd className="wrap-anywhere text-foreground">{dashboardTitle}</dd>
          <dt>Proposal:</dt>
          <dd className="wrap-anywhere text-foreground">{proposalTitle}</dd>
        </dl>
      )}
      {conflict.reason === "invalid" ? (
        <p className="text-xs text-destructive">
          This change cannot be merged as it is
        </p>
      ) : (
        <RadioGroup
          aria-label={label}
          value={value ?? ""}
          disabled={disabled}
          onValueChange={(next) => onChange(next as Side)}
        >
          {sides.map((side) => (
            <label
              key={side.value}
              className="flex items-center gap-2 text-[13px]"
            >
              <RadioGroupItem value={side.value} />
              {side.label}
            </label>
          ))}
        </RadioGroup>
      )}
    </div>
  )
}
