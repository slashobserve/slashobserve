import { useEffect, useMemo } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys, observeKeys, threadKeys } from "@/api/keys"
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

export type SentMessage = {
  thread: Thread
  message: Message
  run: NonNullable<Message["run"]> | null
  runRefusal: NonNullable<Message["run_refusal"]> | null
}

function sentMessage(body: { thread: Thread; message: Message }): SentMessage {
  return {
    thread: body.thread,
    message: body.message,
    run: body.message.run ?? null,
    runRefusal: body.message.run_refusal ?? null,
  }
}

export function askedAgent(sent: SentMessage): boolean {
  return sent.run !== null || sent.runRefusal !== null
}

const dashboardTarget = "dashboard:"

function dashboardIdOf(target: string): string | null {
  return target.startsWith(dashboardTarget)
    ? target.slice(dashboardTarget.length)
    : null
}

function refreshAfterSend(
  queryClient: QueryClient,
  target: string,
  sent: SentMessage
) {
  void queryClient.invalidateQueries({ queryKey: threadKeys.list(target) })
  const dashboardId = dashboardIdOf(target)
  if (dashboardId !== null) {
    void queryClient.invalidateQueries({
      queryKey: dashboardKeys.comments(dashboardId),
    })
  }
  if (sent.run === null) return
  if (dashboardId !== null) {
    void queryClient.invalidateQueries({
      queryKey: dashboardKeys.revisions(dashboardId),
    })
  }
  void queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() })
}

export function useOpenThread(target: string) {
  const queryClient = useQueryClient()
  const dashboardId = dashboardIdOf(target)
  return useMutation({
    mutationKey:
      dashboardId === null ? undefined : dashboardKeys.posts(dashboardId),
    networkMode: "always",
    mutationFn: (input: {
      body: string
      startRuns: boolean
      version?: number
      panelId?: string | null
    }) =>
      api
        .POST("/threads", {
          body: {
            target,
            body: input.body,
            ...(input.version === undefined ? {} : { version: input.version }),
            ...(input.panelId == null ? {} : { panel_id: input.panelId }),
            ...(input.startRuns ? { start_runs: true } : {}),
          },
        })
        .then(unwrap)
        .then(sentMessage),
    onSuccess: (sent) => refreshAfterSend(queryClient, target, sent),
  })
}

export function useReplyToThread(target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: {
      threadId: string
      body: string
      startRuns: boolean
    }) =>
      api
        .POST("/threads/{thread_id}/messages", {
          params: { path: { thread_id: input.threadId } },
          body: {
            body: input.body,
            ...(input.startRuns ? { start_runs: true } : {}),
          },
        })
        .then(unwrap)
        .then(sentMessage),
    onSuccess: (sent) => refreshAfterSend(queryClient, target, sent),
  })
}
