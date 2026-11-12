import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { useSetGrantMode } from "./queries"
import {
  agentName,
  modeChoices,
  type ConnectedAgent,
  type GrantMode,
} from "./rows"

const items = modeChoices.map(({ value, label }) => ({ value, label }))

export function ModeSelect({ grant }: { grant: ConnectedAgent }) {
  const setMode = useSetGrantMode()
  return (
    <Select
      items={items}
      value={grant.mode}
      onValueChange={(next: GrantMode | null) => {
        if (next === null || next === grant.mode) return
        setMode.mutate({ id: grant.id, mode: next })
      }}
    >
      <SelectTrigger
        size="sm"
        aria-label={`Mode for ${agentName(grant.client.name)}`}
        aria-busy={setMode.isPending || undefined}
        disabled={setMode.isPending}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {modeChoices.map((choice) => (
          <SelectItem key={choice.value} value={choice.value}>
            <span className="flex flex-col gap-0.5">
              <span>{choice.label}</span>
              <span className="text-xs text-muted-foreground">
                {choice.description}
              </span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
