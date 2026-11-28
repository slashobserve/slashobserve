import { useMutation, useQueryClient } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { annotationKeys } from "@/api/keys"

import type { CreateBody, UpdateBody } from "./form"

function useRefreshAnnotations() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: annotationKeys.all })
}

export function useCreateAnnotation() {
  const refresh = useRefreshAnnotations()
  return useMutation({
    mutationFn: (body: CreateBody) =>
      api.POST("/annotations", { body }).then(unwrap),
    onSuccess: refresh,
  })
}

export function useUpdateAnnotation(id: string) {
  const refresh = useRefreshAnnotations()
  return useMutation({
    mutationFn: (body: UpdateBody) =>
      api
        .PATCH("/annotations/{id}", { params: { path: { id } }, body })
        .then(unwrap),
    onSuccess: refresh,
  })
}

export function useDeleteAnnotation(id: string) {
  const refresh = useRefreshAnnotations()
  return useMutation({
    mutationFn: () =>
      api
        .DELETE("/annotations/{id}", { params: { path: { id } } })
        .then(unwrap),
    onSuccess: refresh,
  })
}
