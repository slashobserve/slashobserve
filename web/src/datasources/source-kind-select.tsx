import type { RefObject } from "react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { datasourceTypes, isDatasourceType, type DatasourceType } from "./draft"

export type SourceKind = DatasourceType | "mcp"

const mcpKind = { value: "mcp", label: "MCP server" } as const

const kinds: { value: SourceKind; label: string }[] = [
  ...datasourceTypes,
  mcpKind,
]

function isSourceKind(value: string): value is SourceKind {
  return value === mcpKind.value || isDatasourceType(value)
}

type Props = {
  id: string
  value: SourceKind | ""
  onChange: (kind: SourceKind) => void
  triggerRef?: RefObject<HTMLElement | null>
  invalid?: boolean
  describedBy?: string
}

export function SourceKindSelect({
  id,
  value,
  onChange,
  triggerRef,
  invalid,
  describedBy,
}: Props) {
  return (
    <Select
      items={kinds}
      value={value === "" ? null : value}
      onValueChange={(pick) => {
        if (pick !== null && isSourceKind(pick)) onChange(pick)
      }}
    >
      <SelectTrigger
        id={id}
        ref={triggerRef as RefObject<HTMLButtonElement> | undefined}
        className="w-full narrow:h-11"
        aria-invalid={invalid}
        aria-describedby={describedBy}
      >
        <SelectValue placeholder="Choose a type" />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {kinds.map((kind) => (
          <SelectItem key={kind.value} value={kind.value}>
            {kind.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
