import { useMyRequests } from "@/observe/queries"

import { newestChats } from "./chat-list"

const fetched = 20

export function useRecentChats(limit: number) {
  const { requests, query } = useMyRequests({}, fetched)
  const chats =
    query.data === undefined ? undefined : newestChats(requests, limit)
  return { chats, query }
}
