import type { InfiniteData } from "@tanstack/react-query"

import type { components } from "@/api/schema.gen"

import type { InboxItem } from "./inbox-item"

export type InboxPages = InfiniteData<
  components["schemas"]["InboxList"],
  string | undefined
>

export type InboxChange =
  | { kind: "readThrough"; seq: number }
  | { kind: "unread"; seq: number; unread: boolean }
  | { kind: "done"; seq: number; done: boolean }

const unreadCap = 100

function countsForYou(item: InboxItem): boolean {
  return item.unread && item.reason !== null
}

function read(item: InboxItem): InboxItem {
  return {
    ...item,
    unread: false,
    state: item.state === "done" ? "done" : "read",
  }
}

function changedItem(item: InboxItem, change: InboxChange): InboxItem {
  if (change.kind === "readThrough") {
    return item.seq <= change.seq ? read(item) : item
  }
  if (item.seq !== change.seq) return item
  switch (change.kind) {
    case "unread":
      return change.unread
        ? { ...item, unread: true, state: "unread" }
        : read(item)
    case "done":
      return change.done
        ? { ...item, unread: false, state: "done" }
        : { ...item, state: item.state === "done" ? "read" : item.state }
  }
}

function itemsOf(lists: readonly InboxPages[]): InboxItem[] {
  return lists.flatMap((list) => list.pages.flatMap((page) => page.items))
}

export function unreadAfter(
  count: number,
  lists: readonly InboxPages[],
  change: InboxChange
): number {
  if (change.kind === "readThrough") {
    const newer = new Set(
      itemsOf(lists)
        .filter((item) => item.seq > change.seq && countsForYou(item))
        .map((item) => item.seq)
    )
    return Math.min(count, newer.size)
  }
  const before = itemsOf(lists).find((item) => item.seq === change.seq)
  if (before === undefined || before.reason === null) return count
  const delta =
    Number(countsForYou(changedItem(before, change))) -
    Number(countsForYou(before))
  if (delta < 0 && count >= unreadCap) return count
  return Math.min(unreadCap, Math.max(0, count + delta))
}

export function applyInboxChange(
  list: InboxPages,
  change: InboxChange,
  unreadCount: number
): InboxPages {
  return {
    ...list,
    pages: list.pages.map((page) => ({
      ...page,
      items: page.items.map((item) => changedItem(item, change)),
      unread_count: unreadCount,
    })),
  }
}
