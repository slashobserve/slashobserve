import { useEffect, useMemo } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryKey,
} from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Comment = components["schemas"]["Comment"]
export type CommentStatus = Comment["status"]

export type PanelThread = Comment & { parent_id: null; panel_id: string }

export function isPanelThread(comment: Comment): comment is PanelThread {
  return comment.parent_id === null && comment.panel_id !== null
}

type CommentsPage = { comments: Comment[]; next_cursor: string | null }
type CommentsData = InfiniteData<CommentsPage, string | undefined>

const pageSize = 500

export function commentsQuery(id: string) {
  return infiniteQueryOptions({
    queryKey: dashboardKeys.comments(id),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/dashboards/{id}/comments", {
          params: {
            path: { id },
            query: { status: "all", limit: pageSize, cursor: pageParam },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 30_000,
  })
}

export function useComments(id: string) {
  const query = useInfiniteQuery(commentsQuery(id))
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
  const comments = useMemo(() => {
    const seen = new Set<string>()
    const unique: Comment[] = []
    for (const page of query.data?.pages ?? []) {
      for (const comment of page.comments) {
        if (seen.has(comment.id)) continue
        seen.add(comment.id)
        unique.push(comment)
      }
    }
    return unique
  }, [query.data])
  return {
    comments,
    isPending: query.isPending,
    isError: query.isError,
    refetch: query.refetch,
    hasNextPage,
    fetchNextPage,
  }
}

function mapComment(
  data: CommentsData | undefined,
  commentId: string,
  change: (comment: Comment) => Comment
): CommentsData | undefined {
  if (!data) return data
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      comments: page.comments.map((comment) =>
        comment.id === commentId ? change(comment) : comment
      ),
    })),
  }
}

export function useSetCommentStatus(
  id: string,
  onSaved?: (status: CommentStatus) => void
) {
  const queryClient = useQueryClient()
  const key = dashboardKeys.comments(id)
  return useMutation({
    mutationFn: (input: { commentId: string; status: CommentStatus }) =>
      api
        .PATCH("/dashboards/{id}/comments/{comment_id}", {
          params: { path: { id, comment_id: input.commentId } },
          body: { status: input.status },
        })
        .then(unwrap)
        .then((body) => body.comment),
    onSuccess: (comment, input) => {
      queryClient.setQueryData<CommentsData>(key, (data) =>
        mapComment(data, input.commentId, () => comment)
      )
      onSaved?.(input.status)
    },
    onError: () => toast.error("Could not update the comment"),
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}

export function useEditMessage(queryKey: QueryKey) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
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
