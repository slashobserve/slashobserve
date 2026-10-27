import { useId, type ComponentProps, type ReactNode } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

export type Option = { value: string; label: string }

export function NativeSelect({
  className,
  ...props
}: ComponentProps<"select">) {
  return (
    <select
      className={cn(
        "min-h-8 min-w-0 rounded-control border border-input bg-background px-2.5 text-[13px] font-normal text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 narrow:text-base",
        className
      )}
      {...props}
    />
  )
}

export function Field({
  label,
  htmlFor,
  children,
}: {
  label: string
  htmlFor: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={htmlFor} className="text-[13px]">
        {label}
      </Label>
      {children}
    </div>
  )
}

export function SelectField({
  label,
  value,
  options,
  onChange,
  disabled,
}: {
  label: string
  value: string
  options: readonly Option[]
  onChange: (value: string) => void
  disabled: boolean
}) {
  const id = useId()
  const known = options.some((option) => option.value === value)
  return (
    <Field label={label} htmlFor={id}>
      <NativeSelect
        id={id}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      >
        {known ? null : <option value={value}>{value}</option>}
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </NativeSelect>
    </Field>
  )
}

export function CheckboxField({
  label,
  checked,
  onChange,
  disabled,
}: {
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
  disabled: boolean
}) {
  return (
    <Label className="gap-2 text-[13px] font-normal">
      <Checkbox
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
      {label}
    </Label>
  )
}

export function CheckboxGroup({
  label,
  options,
  selected,
  max,
  onToggle,
  disabled,
}: {
  label: string
  options: readonly Option[]
  selected: readonly string[]
  max?: number
  onToggle: (value: string, on: boolean) => void
  disabled: boolean
}) {
  const full = max !== undefined && selected.length >= max
  return (
    <fieldset className="m-0 flex min-w-0 flex-col gap-1.5 border-0 p-0">
      <legend className="mb-1.5 p-0 text-[13px] font-medium">{label}</legend>
      {options.map((option) => {
        const checked = selected.includes(option.value)
        return (
          <CheckboxField
            key={option.value}
            label={option.label}
            checked={checked}
            disabled={disabled || (full && !checked)}
            onChange={(on) => onToggle(option.value, on)}
          />
        )
      })}
    </fieldset>
  )
}
