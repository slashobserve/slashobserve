import { useEffect, useMemo } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
  type QueryKey,
} from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { resendOnceOffline, unwrap } from "@/api/errors"
import { dashboardKeys, observeKeys, threadKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Thread = components["schemas"]["Thread"]
export type Message = components["schemas"]["Message"]
export type ThreadPage = components["schemas"]["ThreadPage"]
export type Anchor = components["schemas"]["Anchor"]
export type ThreadWithMessages = { thread: Thread; messages: Message[] }

const pageSize = 500

export function openThreadCount(
  threads: readonly ThreadWithMessages[]
): number {
  return threads.filter(({ thread }) => thread.status === "open").length
}

export function openThreadsByPanel(
  threads: readonly ThreadWithMessages[]
): Map<string, number> {
  const counts = new Map<string, number>()
  for (const { thread } of threads) {
    if (thread.status !== "open" || thread.panel_id === null) continue
    counts.set(thread.panel_id, (counts.get(thread.panel_id) ?? 0) + 1)
  }
  return counts
}

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
  to: ThreadSent["to"]
  replayed: boolean
}

type ThreadSent = components["schemas"]["ThreadSent"]

function sentMessage(body: ThreadSent): SentMessage {
  return {
    thread: body.thread,
    message: body.message,
    to: body.to,
    run: body.message.run ?? null,
    runRefusal: body.message.run_refusal ?? null,
    replayed: body.replayed,
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
    mutationFn: (input: {
      body: string
      to?: "agent" | "everyone"
      clientId?: string
      version?: number
      panelId?: string | null
      blockId?: string | null
      anchor?: Anchor
      timeZone?: string
    }) =>
      resendOnceOffline(input.clientId, () =>
        api.POST("/threads", {
          body: {
            target,
            body: input.body,
            ...(input.to === undefined ? {} : { to: input.to }),
            ...(input.clientId === undefined
              ? {}
              : { client_id: input.clientId }),
            ...(input.version === undefined ? {} : { version: input.version }),
            ...(input.panelId == null ? {} : { panel_id: input.panelId }),
            ...(input.blockId == null ? {} : { block_id: input.blockId }),
            ...(input.anchor === undefined ? {} : { anchor: input.anchor }),
            ...(input.timeZone === undefined
              ? {}
              : { time_zone: input.timeZone }),
          },
        })
      )
        .then(unwrap)
        .then(sentMessage),
    onSuccess: (sent) => refreshAfterSend(queryClient, target, sent),
  })
}

export function useReplyToThread(target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      threadId: string
      body: string
      to?: "agent" | "everyone"
      clientId?: string
      blockId?: string | null
      replyToId?: string | null
      timeZone?: string
    }) =>
      resendOnceOffline(input.clientId, () =>
        api.POST("/threads/{thread_id}/messages", {
          params: { path: { thread_id: input.threadId } },
          body: {
            body: input.body,
            ...(input.to === undefined ? {} : { to: input.to }),
            ...(input.clientId === undefined
              ? {}
              : { client_id: input.clientId }),
            ...(input.blockId == null ? {} : { block_id: input.blockId }),
            ...(input.replyToId == null
              ? {}
              : { reply_to_id: input.replyToId }),
            ...(input.timeZone === undefined
              ? {}
              : { time_zone: input.timeZone }),
          },
        })
      )
        .then(unwrap)
        .then(sentMessage),
    onSuccess: (sent) => refreshAfterSend(queryClient, target, sent),
  })
}

export function useApplySuggestedChange(target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { threadId: string; findingId: string }) =>
      api
        .POST("/threads/{thread_id}/apply", {
          params: { path: { thread_id: input.threadId } },
          body: { finding_id: input.findingId },
        })
        .then(unwrap)
        .then(sentMessage),
    onSuccess: (sent) => refreshAfterSend(queryClient, target, sent),
  })
}

function mapThread(
  data: InfiniteData<ThreadPage, string | undefined> | undefined,
  threadId: string,
  change: (thread: Thread) => Thread
) {
  if (data === undefined) return data
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      threads: page.threads.map((thread) =>
        thread.id === threadId ? change(thread) : thread
      ),
    })),
  }
}

export function useSetThreadStatus(target: string) {
  const queryClient = useQueryClient()
  const key = threadKeys.list(target)
  return useMutation({
    mutationFn: (input: { threadId: string; status: Thread["status"] }) =>
      (input.status === "resolved"
        ? api.POST("/threads/{thread_id}/resolve", {
            params: { path: { thread_id: input.threadId } },
          })
        : api.POST("/threads/{thread_id}/reopen", {
            params: { path: { thread_id: input.threadId } },
          })
      )
        .then(unwrap)
        .then((body) => body.thread),
    onSuccess: (thread) =>
      queryClient.setQueryData<InfiniteData<ThreadPage, string | undefined>>(
        key,
        (data) => mapThread(data, thread.id, () => thread)
      ),
    onError: () => toast.error("Could not update the thread"),
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}

export function useMarkThreadRead(target: string) {
  const queryClient = useQueryClient()
  const key = threadKeys.list(target)
  return useMutation({
    mutationFn: (threadId: string) =>
      api
        .POST("/threads/{thread_id}/read", {
          params: { path: { thread_id: threadId } },
        })
        .then(unwrap),
    onSuccess: (_, threadId) =>
      queryClient.setQueryData<InfiniteData<ThreadPage, string | undefined>>(
        key,
        (data) =>
          mapThread(data, threadId, (thread) => ({ ...thread, unread: false }))
      ),
  })
}

export function useDeleteMessage(queryKey: QueryKey) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (messageId: string) =>
      api
        .DELETE("/messages/{message_id}", {
          params: { path: { message_id: messageId } },
        })
        .then(unwrap),
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  })
}

export function useEditMessage(queryKey: QueryKey) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { messageId: string; body: string }) =>
      api
        .PATCH("/messages/{message_id}", {
          params: { path: { message_id: input.messageId } },
          body: { body: input.body },
        })
        .then(unwrap),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  })
}
