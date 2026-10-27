import { ChevronDownIcon, LoaderCircleIcon, RotateCcwIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

const controlButton = "h-auto rounded-none px-[9px] text-[13px]"

type SelectProps = {
  id: string
  label: string
  values: string[]
  selected: string | undefined
  refreshing?: boolean
  optionLabel?: (value: string) => string
  onChange: (value: string) => void
}

export function ValueSelect({
  id,
  label,
  values,
  selected,
  refreshing = false,
  optionLabel = (value) => value,
  onChange,
}: SelectProps) {
  return (
    <Select
      items={values.map((value) => ({ value, label: optionLabel(value) }))}
      value={selected ?? null}
      onValueChange={(next) => {
        if (next !== null) onChange(next)
      }}
      disabled={refreshing || values.length === 0}
    >
      <SelectTrigger
        id={id}
        aria-label={label}
        aria-busy={refreshing || undefined}
        className="min-w-24 gap-1.5 rounded-none border-0 px-[9px] py-0 focus-visible:ring-0 data-[size=default]:h-auto"
        icon={
          refreshing ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="pointer-events-none size-3 text-muted-foreground motion-safe:animate-spin"
            />
          ) : (
            <ChevronDownIcon
              aria-hidden="true"
              className="pointer-events-none size-3 text-muted-foreground"
            />
          )
        }
      >
        <SelectValue placeholder={refreshing ? "Loading..." : "No values"} />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {values.map((value) => (
          <SelectItem key={value} value={value}>
            {optionLabel(value)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function LoadingControl({ id, label }: { id: string; label: string }) {
  return (
    <Button
      variant="ghost"
      disabled
      aria-label={`${label}, loading`}
      id={id}
      className={controlButton}
    >
      Loading...
    </Button>
  )
}

type FailedProps = { id: string; label: string; retry: () => void }

export function FailedControl({ id, label, retry }: FailedProps) {
  return (
    <>
      <Button
        variant="ghost"
        disabled
        aria-label={`${label}, failed to load`}
        id={id}
        className={controlButton}
      >
        Failed to load
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        className="rounded-none"
        aria-label={`Retry loading ${label}`}
        onClick={retry}
      >
        <RotateCcwIcon />
      </Button>
    </>
  )
}
