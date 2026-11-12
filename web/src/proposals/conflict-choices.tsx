import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"

import type { MergeConflict } from "./queries"

export type Side = "ours" | "theirs"

const sides: { value: Side; label: string }[] = [
  { value: "ours", label: "Keep the dashboard's" },
  { value: "theirs", label: "Take this proposal's" },
]

type Props = {
  conflict: MergeConflict
  label: string
  value: Side | undefined
  onChange: (side: Side) => void
  disabled?: boolean
}

export function ConflictChoices({
  conflict,
  label,
  value,
  onChange,
  disabled = false,
}: Props) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[13px] wrap-anywhere">{conflict.message}</p>
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
