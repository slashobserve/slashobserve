import { useId, type ReactNode } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export type Option = { value: string; label: string }

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
  const items = options.some((option) => option.value === value)
    ? options
    : [{ value, label: value }, ...options]
  return (
    <Field label={label} htmlFor={id}>
      <Select
        items={items}
        value={value}
        disabled={disabled}
        onValueChange={(next) => {
          if (next !== null) onChange(next)
        }}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
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
