import { useEffect, useMemo } from "react"
import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Comment = components["schemas"]["Comment"]
export type CommentStatus = Comment["status"]

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

export function useCreateComment(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      version: number
      panelId: string
      body: string
      requestRevision: boolean
    }) =>
      api
        .POST("/dashboards/{id}/comments", {
          params: { path: { id } },
          body: {
            version: input.version,
            panel_id: input.panelId,
            body: input.body,
            request_revision: input.requestRevision,
          },
        })
        .then(unwrap)
        .then((body) => body.comment),
    onSuccess: (comment) => {
      queryClient.setQueryData<CommentsData>(
        dashboardKeys.comments(id),
        (data) => {
          if (!data || data.pages.length === 0) return data
          const known = data.pages.some((page) =>
            page.comments.some((item) => item.id === comment.id)
          )
          if (known) return data
          const last = data.pages.length - 1
          return {
            ...data,
            pages: data.pages.map((page, index) =>
              index === last
                ? { ...page, comments: [...page.comments, comment] }
                : page
            ),
          }
        }
      )
      void queryClient.invalidateQueries({
        queryKey: dashboardKeys.comments(id),
      })
    },
  })
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
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient
        .getQueryData<CommentsData>(key)
        ?.pages.flatMap((page) => page.comments)
        .find((comment) => comment.id === input.commentId)
      queryClient.setQueryData<CommentsData>(key, (data) =>
        mapComment(data, input.commentId, (comment) => ({
          ...comment,
          status: input.status,
          resolved_at:
            input.status === "resolved" ? new Date().toISOString() : null,
          resolved_by: input.status === "resolved" ? comment.resolved_by : null,
        }))
      )
      return { previous }
    },
    onSuccess: (_comment, input) => onSaved?.(input.status),
    onError: (_error, input, context) => {
      const previous = context?.previous
      if (previous) {
        queryClient.setQueryData<CommentsData>(key, (data) =>
          mapComment(data, input.commentId, () => previous)
        )
      }
      toast.error("Could not update the comment")
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}
