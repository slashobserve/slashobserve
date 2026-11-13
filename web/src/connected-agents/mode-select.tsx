import { useQueryClient } from "@tanstack/react-query"

import { agentKeys } from "@/api/keys"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { useSetGrantMode } from "./queries"
import { modeChoices, type GrantMode } from "./rows"

const items = modeChoices.map(({ value, label }) => ({ value, label }))

type Props = { grantId: string; mode: GrantMode; name: string }

export function ModeSelect({ grantId, mode, name }: Props) {
  const queryClient = useQueryClient()
  const setMode = useSetGrantMode()
  return (
    <Select
      items={items}
      value={mode}
      onValueChange={(next: GrantMode | null) => {
        if (next === null || next === mode) return
        setMode.mutate(
          { id: grantId, mode: next },
          {
            onSettled: () =>
              queryClient.invalidateQueries({ queryKey: agentKeys.all }),
          }
        )
      }}
    >
      <SelectTrigger
        size="sm"
        aria-label={`Mode for ${name}`}
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
