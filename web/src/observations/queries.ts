import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import {
  observationKeys,
  threadKeys,
  type ObservationListFilters,
} from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { touchesObservation } from "@/live/activity-match"
import {
  useActivityStream,
  useDocumentVisible,
} from "@/live/use-activity-stream"

import { BlockVersionConflict, unwrapBlockWrite } from "./errors"

export type Observation = components["schemas"]["Observation"]
export type Block = components["schemas"]["ObservationBlock"]
export type ObservationEnvelope = components["schemas"]["ObservationEnvelope"]

type BlockBody = { [key: string]: unknown }
type BlockWidth = components["schemas"]["ObservationBlockWidth"]

export function replaceBlock(
  envelope: ObservationEnvelope,
  block: Block
): ObservationEnvelope {
  const blocks = envelope.observation.blocks
  if (blocks === undefined) return envelope
  return {
    ...envelope,
    observation: {
      ...envelope.observation,
      blocks: blocks.map((existing) =>
        existing.id === block.id ? block : existing
      ),
    },
  }
}

export function withoutBlock(
  envelope: ObservationEnvelope,
  blockId: string
): ObservationEnvelope {
  const blocks = envelope.observation.blocks
  if (blocks === undefined) return envelope
  return {
    ...envelope,
    observation: {
      ...envelope.observation,
      blocks: blocks.filter((block) => block.id !== blockId),
    },
  }
}

export function keepKnownBlocks(
  previous: ObservationEnvelope | undefined,
  next: ObservationEnvelope
): ObservationEnvelope {
  const blocks = previous?.observation.blocks
  if (next.observation.blocks !== undefined || blocks === undefined) {
    return next
  }
  return { ...next, observation: { ...next.observation, blocks } }
}

export function observationQuery(id: string) {
  return queryOptions({
    queryKey: observationKeys.one(id),
    queryFn: ({ signal }) =>
      api
        .GET("/observations/{id}", { params: { path: { id } }, signal })
        .then(unwrap),
  })
}

export function useObservation(id: string) {
  return useQuery(observationQuery(id))
}

export function observationsInfiniteQuery(
  filters: ObservationListFilters,
  limit = 20
) {
  return infiniteQueryOptions({
    queryKey: observationKeys.list(filters, limit),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/observations", {
          params: {
            query: {
              limit,
              cursor: pageParam,
              ...(filters.state === undefined ? {} : { state: filters.state }),
              ...(filters.folderId === undefined
                ? {}
                : { folder_id: filters.folderId }),
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}

function writeEnvelope(
  queryClient: QueryClient,
  id: string,
  envelope: ObservationEnvelope
) {
  queryClient.setQueryData<ObservationEnvelope>(
    observationKeys.one(id),
    (previous) => keepKnownBlocks(previous, envelope)
  )
}

function writeBlock(queryClient: QueryClient, id: string, block: Block) {
  queryClient.setQueryData<ObservationEnvelope>(
    observationKeys.one(id),
    (previous) =>
      previous === undefined ? previous : replaceBlock(previous, block)
  )
}

function removeBlockFromCache(
  queryClient: QueryClient,
  id: string,
  blockId: string
) {
  queryClient.setQueryData<ObservationEnvelope>(
    observationKeys.one(id),
    (previous) =>
      previous === undefined ? previous : withoutBlock(previous, blockId)
  )
}

function refreshLists(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: observationKeys.lists() })
}

function refetchObservation(queryClient: QueryClient, id: string) {
  return queryClient.invalidateQueries({
    queryKey: observationKeys.one(id),
    exact: true,
  })
}

export type CreateObservationInput =
  components["schemas"]["CreateObservationRequest"]

export function useCreateObservation() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: CreateObservationInput) =>
      api.POST("/observations", { body: input }).then(unwrap),
    onSuccess: (envelope) => {
      const id = envelope.observation.id
      writeEnvelope(queryClient, id, envelope)
      void refetchObservation(queryClient, id)
    },
    onSettled: () => refreshLists(queryClient),
  })
}

function useObservationMutation<Input>(
  id: string,
  call: (input: Input) => Promise<ObservationEnvelope>
) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: call,
    onSuccess: (envelope) => writeEnvelope(queryClient, id, envelope),
    onSettled: () => refreshLists(queryClient),
  })
}

export function useUpdateObservation(id: string) {
  return useObservationMutation(
    id,
    (input: components["schemas"]["UpdateObservationRequest"]) =>
      api
        .PATCH("/observations/{id}", {
          params: { path: { id } },
          body: input,
        })
        .then(unwrap)
  )
}

export function useMoveObservation(id: string) {
  return useObservationMutation(id, (input: { folder_id: string }) =>
    api
      .POST("/observations/{id}/move", {
        params: { path: { id } },
        body: input,
      })
      .then(unwrap)
  )
}

export function useResolveObservation(id: string) {
  return useObservationMutation(id, (input: { summary: string }) =>
    api
      .POST("/observations/{id}/resolve", {
        params: { path: { id } },
        body: input,
      })
      .then(unwrap)
  )
}

export function useReopenObservation(id: string) {
  return useObservationMutation(id, () =>
    api
      .POST("/observations/{id}/reopen", { params: { path: { id } } })
      .then(unwrap)
  )
}

function useBlockMutation<Input, Result>(
  id: string,
  call: (input: Input) => Promise<Result>,
  onWritten: (queryClient: QueryClient, result: Result, input: Input) => void
) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: call,
    onSuccess: (result, input) => {
      onWritten(queryClient, result, input)
      void refetchObservation(queryClient, id)
    },
    onError: (error) => {
      if (error instanceof BlockVersionConflict) {
        writeBlock(queryClient, id, error.current)
      }
    },
    onSettled: () => refreshLists(queryClient),
  })
}

export function useAddBlock(id: string) {
  return useBlockMutation(
    id,
    (input: {
      kind: "text" | "panel" | "snapshot"
      body?: BlockBody
      width?: BlockWidth
      position?: number
      from_block_id?: string
    }) =>
      api
        .POST("/observations/{id}/blocks", {
          params: { path: { id } },
          body: input,
        })
        .then(unwrap),
    () => {}
  )
}

export function useUpdateBlock(id: string) {
  return useBlockMutation(
    id,
    ({
      blockId,
      ...body
    }: {
      blockId: string
      version: number
      body?: BlockBody
      width?: BlockWidth
      position?: number
    }) =>
      api
        .PATCH("/blocks/{id}", { params: { path: { id: blockId } }, body })
        .then(unwrapBlockWrite),
    (queryClient, { block }) => writeBlock(queryClient, id, block)
  )
}

export function useRemoveBlock(id: string) {
  return useBlockMutation(
    id,
    ({ blockId, version }: { blockId: string; version: number }) =>
      api
        .DELETE("/blocks/{id}", {
          params: { path: { id: blockId }, query: { version } },
        })
        .then(unwrapBlockWrite),
    (queryClient, _, { blockId }) =>
      removeBlockFromCache(queryClient, id, blockId)
  )
}

export function useKeepBlock(id: string) {
  return useBlockMutation(
    id,
    ({ blockId, version }: { blockId: string; version: number }) =>
      api
        .POST("/blocks/{id}/keep", {
          params: { path: { id: blockId } },
          body: { version },
        })
        .then(unwrapBlockWrite),
    (queryClient, { block }) => writeBlock(queryClient, id, block)
  )
}

export function useDiscardBlock(id: string) {
  return useBlockMutation(
    id,
    ({ blockId }: { blockId: string }) =>
      api
        .POST("/blocks/{id}/discard", { params: { path: { id: blockId } } })
        .then(unwrapBlockWrite),
    (queryClient, _, { blockId }) =>
      removeBlockFromCache(queryClient, id, blockId)
  )
}

export function useObservationLive(
  observation: Pick<Observation, "id" | "folder_id" | "thread_id" | "blocks">
) {
  const queryClient = useQueryClient()
  const visible = useDocumentVisible()
  const discussion = threadKeys.list(`observation:${observation.id}`)
  useActivityStream({
    folderId: observation.folder_id,
    enabled: visible,
    onEvent: (event) => {
      const touched = touchesObservation(event, observation)
      if (touched === "observation") {
        void queryClient.invalidateQueries({
          queryKey: observationKeys.one(observation.id),
          exact: true,
        })
      } else if (touched === "discussion") {
        void queryClient.invalidateQueries({ queryKey: discussion })
      }
    },
    onGap: () => {
      void queryClient.invalidateQueries({
        queryKey: observationKeys.one(observation.id),
        exact: true,
      })
      void queryClient.invalidateQueries({ queryKey: discussion })
    },
  })
}
