import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, NetworkError, unwrap } from "@/api/errors"
import { promptKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { sentence } from "@/observe/queries"

import { invalidNameText } from "./rules"

export type SavedPrompt = components["schemas"]["SavedPrompt"]

export function promptsQuery() {
  return queryOptions({
    queryKey: promptKeys.list(),
    queryFn: ({ signal }) => api.GET("/prompts", { signal }).then(unwrap),
    select: (data) => data.prompts,
    staleTime: 30_000,
  })
}

export function useCreatePrompt() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { name: string; body: string; shared: boolean }) =>
      api.POST("/prompts", { body: input }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: promptKeys.all }),
  })
}

export function useUpdatePrompt() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({
      id,
      patch,
    }: {
      id: string
      patch: { name?: string; body?: string; shared?: boolean }
    }) =>
      api
        .PATCH("/prompts/{id}", { params: { path: { id } }, body: patch })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: promptKeys.all }),
  })
}

export function useDeletePrompt() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (id: string) =>
      api
        .DELETE("/prompts/{id}", { params: { path: { id } } })
        .then((result) =>
          result.response.status === 404 ? undefined : unwrap(result)
        ),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: promptKeys.all })
    },
  })
}

export type PromptProblem = { field: "name" | "body" | null; text: string }

export function promptProblem(
  error: unknown,
  attempt: { name: string; shared: boolean }
): PromptProblem {
  if (error instanceof NetworkError) {
    return { field: null, text: "Could not reach the server. Try again." }
  }
  if (!(error instanceof ApiError)) {
    return { field: null, text: "Something went wrong. Try again." }
  }
  if (error.status === 409 && error.code === "name_taken") {
    return {
      field: "name",
      text: attempt.shared
        ? `Another shared prompt is named "${attempt.name}". Pick another name, or keep it to yourself.`
        : `You already have a prompt named "${attempt.name}".`,
    }
  }
  if (error.status === 422 && error.code === "invalid_name") {
    return { field: "name", text: invalidNameText }
  }
  if (error.status === 422 && error.code === "invalid_body") {
    return {
      field: "body",
      text: "Use 1 to 4000 characters, without control characters.",
    }
  }
  if (error.status === 422 && error.code === "too_many_prompts") {
    return {
      field: null,
      text: "You have 100 saved prompts, the most you can keep. Delete one first.",
    }
  }
  if (error.status === 404) {
    return { field: null, text: "This prompt no longer exists." }
  }
  if (error.status === 403) {
    return {
      field: null,
      text:
        error.code === "not_owner"
          ? "Only the owner or an admin can change this prompt."
          : "Your role cannot save prompts.",
    }
  }
  return { field: null, text: sentence(error.message) }
}
