import type { DiscussionEntry, DiscussionShow } from "./discussion-model"

export type DiscussionFilter = { show: DiscussionShow; blockId: string | null }

export type BlockRef = { id: string; title: string }

export const noFilter: DiscussionFilter = { show: "everything", blockId: null }

export function isFiltered(filter: DiscussionFilter): boolean {
  return filter.show !== "everything" || filter.blockId !== null
}

export function filterEntries(
  entries: readonly DiscussionEntry[],
  filter: DiscussionFilter
): DiscussionEntry[] {
  const kept: DiscussionEntry[] = []
  const keptMessages = new Set<string>()
  for (const entry of entries) {
    if (entry.kind === "message") {
      if (filter.show === "changes") continue
      const { message } = entry
      const aboutBlock =
        filter.blockId === null ||
        message.block_id === filter.blockId ||
        (message.reply_to !== null && keptMessages.has(message.reply_to.id))
      if (!aboutBlock) continue
      keptMessages.add(message.id)
    } else {
      if (filter.show === "messages") continue
      const { subject } = entry.change
      if (filter.blockId !== null && subject?.id !== filter.blockId) continue
    }
    kept.push(entry)
  }
  return kept
}
