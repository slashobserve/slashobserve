import { markOfAuthor } from "@/lib/author"

import { boxClass, EntryFrame } from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "annotation" }>
  panelTitle: string | null
  onSelectPanel: (panelId: string) => void
}

export function AnnotationEntry({ entry, panelTitle, onSelectPanel }: Props) {
  const { annotation } = entry
  const who = annotation.created_by_name ?? "Someone"
  const panelId = annotation.panel_id
  return (
    <EntryFrame
      avatar={markOfAuthor({
        kind: annotation.actor?.kind ?? "user",
        id: annotation.created_by ?? annotation.id,
        name: who,
      })}
      who={who}
      verb="added an annotation"
      panel={
        panelId === null || panelTitle === null
          ? undefined
          : { title: panelTitle, onSelect: () => onSelectPanel(panelId) }
      }
      at={entry.at}
    >
      <div className={`${boxClass} wrap-anywhere`}>
        {annotation.text}
        {annotation.tags.length === 0 ? null : (
          <span className="mt-0.5 block text-xs text-muted-foreground">
            {annotation.tags.join(", ")}
          </span>
        )}
      </div>
    </EntryFrame>
  )
}
