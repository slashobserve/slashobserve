import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import { draftQueryRequest, type DraftAnswer } from "@/ask/draft-query"

import { useAsk } from "./use-ask"

vi.mock("@/ask/draft-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/ask/draft-query")>()),
  draftQueryRequest: vi.fn(),
}))

const drafted: DraftAnswer = {
  query: "",
  language: "traceql",
  traceFilters: { service: "checkout" },
  explanation: "Checkout traces",
  warnings: ["Errors only was left off"],
}

const range = { from: "now-1h", to: "now" }

function abortable(signal: AbortSignal | undefined): Promise<DraftAnswer> {
  return new Promise((_, reject) => {
    signal?.addEventListener("abort", () =>
      reject(new DOMException("Aborted", "AbortError"))
    )
  })
}

function render(overrides: Partial<Parameters<typeof useAsk>[0]> = {}) {
  const onAnswer = vi.fn()
  const onCannotTranslate = vi.fn()
  const hook = renderHook(() =>
    useAsk({
      uid: "tempo",
      signal: "traces",
      range,
      initialQuestion: undefined,
      initialFromQuestion: null,
      onAnswer,
      onCannotTranslate,
      ...overrides,
    })
  )
  return { ...hook, onAnswer, onCannotTranslate }
}

afterEach(() => vi.mocked(draftQueryRequest).mockReset())

describe("useAsk", () => {
  it("passes a resolved draft to onAnswer with the question", async () => {
    vi.mocked(draftQueryRequest).mockResolvedValue(drafted)
    const { result, onAnswer } = render()

    act(() => result.current.ask("slow checkout"))
    expect(result.current.asking).toBe(true)
    await waitFor(() => expect(result.current.asking).toBe(false))

    expect(onAnswer).toHaveBeenCalledWith(drafted, "slow checkout")
    expect(result.current.answer).toEqual({
      explanation: "Checkout traces",
      warnings: ["Errors only was left off"],
    })
    expect(result.current.fromQuestion).toBe("slow checkout")
    expect(result.current.problem).toBeNull()
    expect(draftQueryRequest).toHaveBeenCalledWith(
      "tempo",
      { question: "slow checkout", signal: "traces", ...range },
      expect.any(AbortSignal)
    )
  })

  it("reports a question it cannot translate", async () => {
    vi.mocked(draftQueryRequest).mockRejectedValue(
      new ApiError(422, "cannot_translate", "No trace field matches")
    )
    const { result, onAnswer, onCannotTranslate } = render()

    act(() => result.current.ask("weather"))
    await waitFor(() => expect(result.current.asking).toBe(false))

    expect(result.current.problem).toEqual({
      kind: "cannot_translate",
      reason: "No trace field matches",
    })
    expect(onCannotTranslate).toHaveBeenCalledOnce()
    expect(onAnswer).not.toHaveBeenCalled()
    expect(result.current.answer).toBeNull()
  })

  it("leaves no answer or problem after a cancel", async () => {
    vi.mocked(draftQueryRequest).mockImplementation((_uid, _body, signal) =>
      abortable(signal)
    )
    const { result, onAnswer } = render()

    act(() => result.current.ask("slow checkout"))
    act(() => result.current.cancel())
    await waitFor(() => expect(result.current.asking).toBe(false))

    expect(result.current.answer).toBeNull()
    expect(result.current.problem).toBeNull()
    expect(onAnswer).not.toHaveBeenCalled()
  })

  it("aborts the first question when a second is asked", async () => {
    const signals: AbortSignal[] = []
    vi.mocked(draftQueryRequest).mockImplementation((_uid, body, signal) => {
      if (signal) signals.push(signal)
      return body.question === "second"
        ? Promise.resolve(drafted)
        : abortable(signal)
    })
    const { result, onAnswer } = render()

    act(() => result.current.ask("first"))
    act(() => result.current.ask("second"))
    await waitFor(() => expect(result.current.asking).toBe(false))

    expect(signals[0]?.aborted).toBe(true)
    expect(onAnswer).toHaveBeenCalledOnce()
    expect(onAnswer).toHaveBeenCalledWith(drafted, "second")
    expect(result.current.fromQuestion).toBe("second")
  })

  it("brings the answered question back on askAgain", async () => {
    vi.mocked(draftQueryRequest).mockResolvedValue(drafted)
    const { result } = render()

    act(() => result.current.ask("slow checkout"))
    await waitFor(() =>
      expect(result.current.fromQuestion).toBe("slow checkout")
    )
    act(() => result.current.setQuestion("something else"))
    act(() => result.current.askAgain())

    expect(result.current.question).toBe("slow checkout")
  })
})
