import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { pinKeys } from "@/api/keys"

import { withPin, type Pin, type PinTarget } from "./pins"

export function usePins() {
  return useQuery({
    queryKey: pinKeys.list(),
    queryFn: ({ signal }) =>
      api
        .GET("/stars", { signal })
        .then(unwrap)
        .then((body) => body.items),
    staleTime: 15_000,
  })
}

export function usePinMutation() {
  const queryClient = useQueryClient()
  const key = pinKeys.list()
  return useMutation({
    mutationFn: ({
      target,
      pinned,
    }: {
      target: PinTarget
      pinned: boolean
    }) => {
      const params = { params: { path: { id: target.id } } }
      return (
        pinned
          ? api.PUT("/dashboards/{id}/star", params)
          : api.DELETE("/dashboards/{id}/star", params)
      ).then(unwrap)
    },
    onMutate: async ({ target, pinned }) => {
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<Pin[]>(key)
      queryClient.setQueryData<Pin[]>(key, (items) =>
        withPin(items ?? [], target, pinned, new Date())
      )
      return { previous }
    },
    onError: (_error, { pinned }, context) => {
      queryClient.setQueryData(key, context?.previous)
      toast.error(
        pinned ? "Could not pin the dashboard" : "Could not unpin the dashboard"
      )
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}
