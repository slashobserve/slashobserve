import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"
import type { components } from "@/api/schema.gen"

import {
  BlockVersionConflict,
  blockWriteProblem,
  unwrapBlockWrite,
} from "./errors"

type Block = components["schemas"]["ObservationBlock"]

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

const current: Block = {
  id: "b1",
  observation_id: "o1",
  kind: "text",
  state: "live",
  position: 0,
  width: "full",
  version: 4,
  body: { markdown: "latest" },
  snapshot_id: null,
  run_id: null,
  created_by: author,
  created_at: "2026-11-12T10:00:00Z",
  updated_at: "2026-11-12T10:05:00Z",
  allowed_actions: ["update", "remove"],
}

describe("blockWriteProblem", () => {
  it("reports a version conflict with the latest block", () => {
    const problem = blockWriteProblem(
      new BlockVersionConflict(
        new ApiError(409, "version_conflict", "version conflict"),
        current
      )
    )
    expect(problem).toEqual({
      kind: "conflict",
      current,
      text: "Someone changed this block. The latest is shown; your change was not saved.",
    })
  })

  it("reports a refused transition with the server sentence", () => {
    expect(
      blockWriteProblem(
        new ApiError(409, "invalid_transition", "block is already live")
      )
    ).toEqual({ kind: "refused", text: "Block is already live." })
  })

  it.each(["invalid_block", "too_many_blocks", "snapshot_too_large"])(
    "reports the 422 code %s as invalid with the server message",
    (code) => {
      expect(
        blockWriteProblem(new ApiError(422, code, "text body is too long"))
      ).toEqual({ kind: "invalid", text: "Text body is too long." })
    }
  )

  it.each([
    new ApiError(403, "forbidden_role", "needs edit"),
    new ApiError(404, "not_found", "not found"),
    new ApiError(500, "internal", "boom"),
    new Error("offline"),
    "nope",
  ])("reports anything else as a retry", (error) => {
    expect(blockWriteProblem(error)).toEqual({
      kind: "other",
      text: "The change could not be saved. Try again.",
    })
  })
})

describe("unwrapBlockWrite", () => {
  it("returns the data of a successful write", () => {
    const data = { block: current }
    expect(
      unwrapBlockWrite({ data, response: new Response(null, { status: 200 }) })
    ).toBe(data)
  })

  it("throws a conflict that carries the current block", () => {
    const body = {
      error: "version conflict",
      code: "version_conflict",
      current,
    }
    const attempt = () =>
      unwrapBlockWrite({
        error: body,
        response: new Response(null, { status: 409 }),
      })
    expect(attempt).toThrow(BlockVersionConflict)
    try {
      attempt()
    } catch (error) {
      expect((error as BlockVersionConflict).current).toEqual(current)
    }
  })

  it("throws a plain ApiError for a 409 that is not a version conflict", () => {
    const body = {
      error: "block is already live",
      code: "invalid_transition",
      state: "live",
      allowed_actions: ["update"],
    }
    try {
      unwrapBlockWrite({
        error: body,
        response: new Response(null, { status: 409 }),
      })
      expect.unreachable()
    } catch (error) {
      expect(error).not.toBeInstanceOf(BlockVersionConflict)
      expect(error).toBeInstanceOf(ApiError)
      expect((error as ApiError).code).toBe("invalid_transition")
    }
  })

  it("throws a plain ApiError for a version conflict without the current block", () => {
    try {
      unwrapBlockWrite({
        error: { error: "version conflict", code: "version_conflict" },
        response: new Response(null, { status: 409 }),
      })
      expect.unreachable()
    } catch (error) {
      expect(error).not.toBeInstanceOf(BlockVersionConflict)
      expect((error as ApiError).code).toBe("version_conflict")
    }
  })
})
