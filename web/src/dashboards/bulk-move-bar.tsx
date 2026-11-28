import { useId, useState } from "react"
import { toast } from "sonner"

import { loadProblemText } from "@/api/errors"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { moveChoices } from "@/folders/move-targets"
import type { Folder } from "@/folders/queries"

import { bulkMoveResult, splitByFolder } from "./bulk-move"
import type { DashboardSummary } from "./dashboard-columns"
import { useMoveDashboard } from "./queries"

type Props = {
  selected: readonly DashboardSummary[]
  folders: readonly Folder[]
  onMoved: (movedIds: readonly string[]) => void
  onClear: () => void
}

export function BulkMoveBar({ selected, folders, onMoved, onClear }: Props) {
  const move = useMoveDashboard()
  const labelId = useId()
  const [target, setTarget] = useState<string | null>(null)
  const [moving, setMoving] = useState(false)
  const items = moveChoices(folders, null).map((folder) => ({
    value: folder.id,
    label: folder.title,
  }))
  const folderTitle = items.find((item) => item.value === target)?.label

  async function run() {
    if (target === null || folderTitle === undefined) return
    const { moving: toMove, staying } = splitByFolder(selected, target)
    setMoving(true)
    const outcomes = await Promise.allSettled(
      toMove.map((dashboard) =>
        move.mutateAsync({ id: dashboard.id, folderId: target })
      )
    )
    setMoving(false)
    const failed = outcomes.flatMap((outcome, index) =>
      outcome.status === "rejected"
        ? [
            {
              id: toMove[index]!.id,
              title: toMove[index]!.title,
              reason: loadProblemText(outcome.reason),
            },
          ]
        : []
    )
    const result = bulkMoveResult({
      folder: folderTitle,
      moved: toMove.length - failed.length,
      already: staying.length,
      failed,
    })
    if (result.kind === "success") toast.success(result.text)
    else if (result.kind === "error") toast.error(result.text)
    else toast.info(result.text)
    const failedIds = new Set(failed.map((failure) => failure.id))
    onMoved(
      selected
        .map((dashboard) => dashboard.id)
        .filter((id) => !failedIds.has(id))
    )
  }

  return (
    <div
      role="region"
      aria-label="Selected dashboards"
      className="mb-3 flex flex-wrap items-center gap-2 rounded-card border bg-muted/40 px-3 py-2"
    >
      <span role="status" className="text-[13px] font-medium tabular-nums">
        {`${selected.length} selected`}
      </span>
      <span id={labelId} className="sr-only">
        Move the selected dashboards to this folder
      </span>
      <Select
        items={items}
        value={target}
        disabled={moving}
        onValueChange={setTarget}
      >
        <SelectTrigger aria-labelledby={labelId} className="min-w-48">
          <SelectValue placeholder="Move to folder" />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        disabled={target === null}
        aria-disabled={moving}
        onClick={() => {
          if (!moving) void run()
        }}
      >
        {moving ? "Moving" : "Move"}
      </Button>
      <Button variant="ghost" disabled={moving} onClick={onClear}>
        Clear selection
      </Button>
    </div>
  )
}
