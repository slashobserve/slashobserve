import { useEffect, useMemo } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { threadKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Thread = components["schemas"]["Thread"]
export type Message = components["schemas"]["Message"]
export type ThreadPage = components["schemas"]["ThreadPage"]
export type ThreadWithMessages = { thread: Thread; messages: Message[] }

const pageSize = 500

export function groupThreadPages(
  pages: readonly Pick<ThreadPage, "threads" | "messages">[]
): ThreadWithMessages[] {
  const threads = new Map<string, Thread>()
  const messages = new Map<string, Message>()
  for (const page of pages) {
    for (const thread of page.threads) threads.set(thread.id, thread)
    for (const message of page.messages) messages.set(message.id, message)
  }
  const grouped = new Map<string, ThreadWithMessages>()
  for (const thread of threads.values()) {
    grouped.set(thread.id, { thread, messages: [] })
  }
  for (const message of messages.values()) {
    grouped.get(message.thread_id)?.messages.push(message)
  }
  const byCreation = (
    a: { created_at: string; id: string },
    b: { created_at: string; id: string }
  ) =>
    a.created_at === b.created_at
      ? a.id.localeCompare(b.id)
      : a.created_at.localeCompare(b.created_at)
  return [...grouped.values()]
    .map((entry) => ({ ...entry, messages: entry.messages.sort(byCreation) }))
    .sort((a, b) => byCreation(a.thread, b.thread))
}

function threadsQuery(target: string) {
  return infiniteQueryOptions({
    queryKey: threadKeys.list(target),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/threads", {
          params: {
            query: {
              target,
              status: "all",
              limit: pageSize,
              cursor: pageParam,
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 30_000,
  })
}

export function useThreads(target: string) {
  const query = useInfiniteQuery(threadsQuery(target))
  const {
    hasNextPage,
    isFetchingNextPage,
    isFetchNextPageError,
    fetchNextPage,
  } = query
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage && !isFetchNextPageError) {
      void fetchNextPage({ cancelRefetch: false })
    }
  }, [hasNextPage, isFetchingNextPage, isFetchNextPageError, fetchNextPage])
  const threads = useMemo(
    () => groupThreadPages(query.data?.pages ?? []),
    [query.data]
  )
  return {
    threads,
    canComment: query.data?.pages[0]?.can_comment ?? false,
    isPending: query.isPending,
    isError: query.isError,
    error: query.error,
    refetch: query.refetch,
    isFetching: query.isFetching,
  }
}

export function useOpenThread(target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { body: string }) =>
      api.POST("/threads", { body: { target, body: input.body } }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: threadKeys.list(target) }),
  })
}

export function useReplyToThread(target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { threadId: string; body: string }) =>
      api
        .POST("/threads/{thread_id}/messages", {
          params: { path: { thread_id: input.threadId } },
          body: { body: input.body },
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: threadKeys.list(target) }),
  })
}

export function useSetThreadStatus(target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { threadId: string; status: Thread["status"] }) => {
      const params = { params: { path: { thread_id: input.threadId } } }
      return (
        input.status === "resolved"
          ? api.POST("/threads/{thread_id}/resolve", params)
          : api.POST("/threads/{thread_id}/reopen", params)
      ).then(unwrap)
    },
    onError: () => toast.error("Could not update the thread"),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: threadKeys.list(target) }),
  })
}
