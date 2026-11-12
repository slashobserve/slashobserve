import { AuthorMark } from "@/components/author-mark"
import { markOf, type Actor } from "@/lib/author"
import type { Block } from "@/observations/queries"

export function bylineText(
  block: Pick<Block, "created_by">,
  keptBy?: Actor
): string {
  if (block.created_by.run_id === null) {
    return `Added by ${block.created_by.label}`
  }
  if (keptBy !== undefined) {
    return `From Observe Agent, added by ${keptBy.name}`
  }
  return "From Observe Agent"
}

export function BlockByline({ block }: { block: Block }) {
  return (
    <span className="flex min-w-0 items-center gap-2 text-[12px] text-muted-foreground">
      <AuthorMark mark={markOf(block.created_by)} size={20} />
      <span className="truncate">{bylineText(block)}</span>
    </span>
  )
}
