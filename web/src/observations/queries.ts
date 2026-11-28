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
  annotationKeys,
  chatKeys,
  dashboardKeys,
  observationKeys,
  observeKeys,
  threadKeys,
  type ObservationListFilters,
  type ObservationPageParams,
} from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { touchesObservation } from "@/live/activity-match"
import { useActivitySubscription } from "@/live/activity-stream"

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
  const before = blocks.find((existing) => existing.id === block.id)?.position
  return {
    ...envelope,
    observation: {
      ...envelope.observation,
      blocks: blocks.map((existing) =>
        existing.id === block.id
          ? block
          : shiftedBy(existing, before, block.position)
      ),
    },
  }
}

function shiftedBy(
  existing: Block,
  from: number | undefined,
  to: number
): Block {
  if (from === undefined || from === to) return existing
  const here = existing.position
  if (to < from && here >= to && here < from) {
    return { ...existing, position: here + 1 }
  }
  if (to > from && here > from && here <= to) {
    return { ...existing, position: here - 1 }
  }
  return existing
}

function replaceBlocks(
  envelope: ObservationEnvelope,
  blocks: readonly Block[]
): ObservationEnvelope {
  if (envelope.observation.blocks === undefined) return envelope
  return {
    ...envelope,
    observation: { ...envelope.observation, blocks: [...blocks] },
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
              ...(pageParam === undefined ? {} : { cursor: pageParam }),
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

export function observationsPageQuery(params: ObservationPageParams) {
  return queryOptions({
    queryKey: observationKeys.page(params),
    queryFn: ({ signal }) =>
      api
        .GET("/observations", {
          params: {
            query: {
              limit: params.size,
              offset: (params.page - 1) * params.size,
              sort: params.sort,
              order: params.order,
              ...(params.state === undefined ? {} : { state: params.state }),
              ...(params.q === undefined || params.q === ""
                ? {}
                : { q: params.q }),
              ...(params.mine === true ? { created_by: "me" as const } : {}),
            },
          },
          signal,
        })
        .then(unwrap),
    placeholderData: (previous) => previous,
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

function writeLayout(
  queryClient: QueryClient,
  id: string,
  blocks: readonly Block[]
) {
  queryClient.setQueryData<ObservationEnvelope>(
    observationKeys.one(id),
    (previous) =>
      previous === undefined ? previous : replaceBlocks(previous, blocks)
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
    mutationFn: call,
    onSuccess: (envelope) => writeEnvelope(queryClient, id, envelope),
    onSettled: () => refreshLists(queryClient),
  })
}

export function useUpdateObservation(id: string) {
  const queryClient = useQueryClient()
  return useObservationMutation(
    id,
    (input: components["schemas"]["UpdateObservationRequest"]) =>
      api
        .PATCH("/observations/{id}", {
          params: { path: { id } },
          body: input,
        })
        .then(unwrap)
        .then((envelope) => {
          if (envelope.observation.chat_rename === "renamed") {
            void queryClient.invalidateQueries({
              queryKey: observeKeys.mineAll(),
            })
          }
          return envelope
        })
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

export function usePinObservation(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/observations/{id}/pin", {
          params: { path: { id } },
          body: {},
        })
        .then(unwrap),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: observationKeys.one(id) })
      void queryClient.invalidateQueries({ queryKey: annotationKeys.all })
    },
  })
}

export function useMakeDashboard(target: {
  observationId: string
  blockId?: string
}) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      target.blockId === undefined
        ? api
            .POST("/observations/{id}/dashboard", {
              params: { path: { id: target.observationId } },
              body: {},
            })
            .then(unwrap)
        : api
            .POST("/blocks/{id}/dashboard", {
              params: { path: { id: target.blockId } },
              body: {},
            })
            .then(unwrap),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.titles() })
      void queryClient.invalidateQueries({ queryKey: chatKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: chatKeys.details() })
    },
  })
}

export function useDeleteObservation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api
        .DELETE("/observations/{id}", { params: { path: { id } } })
        .then(unwrap),
    onSuccess: (_, id) => {
      queryClient.removeQueries({
        queryKey: observationKeys.one(id),
        exact: true,
      })
    },
    onSettled: () => refreshLists(queryClient),
  })
}

export function useResolveObservations() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, summary }: { id: string; summary: string }) =>
      api
        .POST("/observations/{id}/resolve", {
          params: { path: { id } },
          body: { summary },
        })
        .then(unwrap),
    onSuccess: (envelope, { id }) => writeEnvelope(queryClient, id, envelope),
    onSettled: () => refreshLists(queryClient),
  })
}

export function useReopenObservationRow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api
        .POST("/observations/{id}/reopen", { params: { path: { id } } })
        .then(unwrap),
    onSuccess: (envelope, id) => writeEnvelope(queryClient, id, envelope),
    onSettled: () => refreshLists(queryClient),
  })
}

function useBlockMutation<Input, Result>(
  id: string | ((input: Input) => string),
  call: (input: Input) => Promise<Result>,
  onWritten: (queryClient: QueryClient, result: Result, input: Input) => void
) {
  const queryClient = useQueryClient()
  const observationOf = (input: Input) =>
    typeof id === "string" ? id : id(input)
  return useMutation({
    mutationFn: call,
    onSuccess: (result, input) => {
      onWritten(queryClient, result, input)
      void refetchObservation(queryClient, observationOf(input))
    },
    onError: (error, input) => {
      if (error instanceof BlockVersionConflict) {
        writeBlock(queryClient, observationOf(input), error.current)
      }
    },
    onSettled: () => refreshLists(queryClient),
  })
}

export type AddBlockInput = {
  kind: "text" | "panel" | "snapshot"
  body?: BlockBody
  width?: BlockWidth
  position?: number
  from_block_id?: string
}

function postBlock(id: string, input: AddBlockInput) {
  return api
    .POST("/observations/{id}/blocks", {
      params: { path: { id } },
      body: input,
    })
    .then(unwrap)
}

export function useAddBlock(id: string) {
  return useBlockMutation(
    id,
    (input: AddBlockInput) => postBlock(id, input),
    () => {}
  )
}

export function useAddBlockTo() {
  return useBlockMutation(
    ({ observationId }: AddBlockInput & { observationId: string }) =>
      observationId,
    ({ observationId, ...input }) => postBlock(observationId, input),
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
      follow_page?: boolean
    }) =>
      api
        .PATCH("/blocks/{id}", { params: { path: { id: blockId } }, body })
        .then(unwrapBlockWrite),
    (queryClient, { block }) => writeBlock(queryClient, id, block)
  )
}

export type ArrangeBody = components["schemas"]["ObservationLayoutRequest"]

export function useArrangeBlocks(id: string) {
  return useBlockMutation(
    id,
    (body: ArrangeBody) =>
      api
        .PUT("/observations/{id}/layout", {
          params: { path: { id } },
          body,
        })
        .then(unwrap),
    (queryClient, { blocks }) => writeLayout(queryClient, id, blocks)
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
  const discussion = threadKeys.list(`observation:${observation.id}`)
  useActivitySubscription({
    folderId: observation.folder_id,
    onEvent: (event) => {
      const touched = touchesObservation(event, observation)
      if (touched === "observation" || touched === "both") {
        void queryClient.invalidateQueries({
          queryKey: observationKeys.one(observation.id),
          exact: true,
        })
      }
      if (touched === "discussion" || touched === "both") {
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
